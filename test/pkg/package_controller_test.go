package pkg

import (
	"context"
	"log"
	"path/filepath"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/apache/openwhisk-client-go/whisk"

	resv1 "github.com/ibm/cloud-operators/pkg/lib/resource/v1"

	"github.com/ibm/cloud-functions-operator/pkg/apis"
	ow "github.com/ibm/cloud-functions-operator/pkg/controller/common"
	"github.com/ibm/cloud-functions-operator/pkg/controller/pkg"
	"github.com/ibm/cloud-functions-operator/pkg/injection"
	owtest "github.com/ibm/cloud-functions-operator/test"
)

var (
	c         client.Client
	cfg       *rest.Config
	namespace string
	ctx       context.Context
	wskclient *whisk.Client
	t         *envtest.Environment
	stop      chan struct{}
)

func TestPackage(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	SetDefaultEventuallyTimeout(30 * time.Second)

	RunSpecs(t, "Package Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(logf.ZapLoggerTo(GinkgoWriter, true))

	t = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "config", "crds")},
		ControlPlaneStartTimeout: 2 * time.Minute,
	}
	apis.AddToScheme(scheme.Scheme)

	var err error
	if cfg, err = t.Start(); err != nil {
		log.Fatal(err)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	Expect(pkg.Add(mgr)).NotTo(HaveOccurred())

	stop = owtest.StartTestManager(mgr)

	namespace = owtest.SetupKubeOrDie(cfg, "openwhisk-package-", nil)
	ctx = injection.WithRequest(context.Background(), &reconcile.Request{NamespacedName: types.NamespacedName{Name: "", Namespace: namespace}})
	ctx = injection.WithKubeClient(ctx, c)

	clientset := owtest.GetClientsetOrDie(cfg)
	config := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "secretmessage",
		},
		Data: map[string][]byte{
			"verysecretkey": []byte("verysecretbody"),
		},
	}
	clientset.CoreV1().Secrets(namespace).Create(config)

	owtest.ConfigureOwprops("seed-defaults-owprops", clientset.CoreV1().Secrets(namespace))

	wskclient, err = ow.NewWskClient(ctx, nil)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	close(stop)
	t.Stop()
})

var _ = Describe("package", func() {

	DescribeTable("should be ready",
		func(specfile string, expected whisk.KeyValueArr) {
			pkg := owtest.LoadPackage("testdata/" + specfile)
			obj := owtest.PostInNs(ctx, pkg, true, 0)

			getParameters := func(pkg *whisk.Package) whisk.KeyValueArr {
				return pkg.Parameters
			}

			Eventually(owtest.GetState(ctx, obj)).Should(Equal(resv1.ResourceStateOnline))
			Eventually(owtest.GetPackage(wskclient, pkg.Name)).Should(WithTransform(getParameters, Equal(expected)))
		},

		Entry("parameter from secret", "pk-parametersfrom-secret.yaml",
			whisk.KeyValueArr{whisk.KeyValue{Key: "verysecretkey", Value: "verysecretbody"}}),
	)

})
