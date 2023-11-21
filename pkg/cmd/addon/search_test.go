package addon

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var _ = Describe("search test", func() {
	var streams genericiooptions.IOStreams
	var out *bytes.Buffer
	const (
		testAddonName       = "apecloud-mysql"
		testAddonNotExisted = "fake-addon"
		testIndexDir        = "./testdata"
	)
	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
	})
	It("test search cmd", func() {
		Expect(newSearchCmd(streams)).ShouldNot(BeNil())
	})

	It("test search", func() {
		cmd := newSearchCmd(streams)
		cmd.Run(cmd, []string{testAddonNotExisted})
		Expect(out.String()).Should(Equal("fake-addon addon not found. Please update your index or check the addon name"))
	})

	It("test addon search", func() {
		expect := []struct {
			index     string
			kind      string
			addonName string
			version   string
		}{
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.7.0",
			},
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.8.0-alpha.5",
			},
			{
				"kubeblocks", "Addon", "apecloud-mysql", "0.8.0-alpha.6",
			},
		}
		result, err := searchAddon(testAddonName, testIndexDir)
		Expect(err).Should(Succeed())
		Expect(result).Should(HaveLen(3))
		for i := range result {
			Expect(result[i].index.name).Should(Equal(expect[i].index))
			Expect(result[i].addon.Name).Should(Equal(expect[i].addonName))
			Expect(result[i].addon.Kind).Should(Equal(expect[i].kind))
			Expect(result[i].addon.Labels[constant.AppVersionLabelKey]).Should(Equal(expect[i].version))
		}
	})

})
