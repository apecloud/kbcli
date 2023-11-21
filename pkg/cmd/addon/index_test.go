package addon

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var _ = Describe("index test", func() {
	var streams genericiooptions.IOStreams
	var out *bytes.Buffer
	const (
		defaultIndexName = "kubeblocks"
		testIndexName    = "kb-other"
		testIndexURL     = "unknown"
		testIndexDir     = "./testdata"
	)
	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
		Expect(addDefaultIndex()).Should(Succeed())
	})

	It("test index cmd", func() {
		Expect(newIndexCmd(streams)).ShouldNot(BeNil())
	})

	It("test index add cmd", func() {
		cmd := newIndexAddCmd()
		Expect(cmd).ShouldNot(BeNil())
		Expect(addIndex([]string{defaultIndexName, testIndexURL})).Should(HaveOccurred())
		Expect(addIndex([]string{testIndexName, testIndexURL})).Should(HaveOccurred())
	})

	It("test index delete cmd", func() {
		Expect(newIndexDeleteCmd()).ShouldNot(BeNil())
		Expect(deleteIndex(testIndexName)).Should(HaveOccurred())
	})

	It("test index list cmd", func() {
		Expect(newIndexListCmd(streams)).ShouldNot(BeNil())
		Expect(listIndexes(out)).Should(Succeed())
		expect := `INDEX        URL                                           
kubeblocks   https://github.com/apecloud/block-index.git   
`
		Expect(out.String()).Should(Equal(expect))
	})

	It("test index update cmd", func() {
		Expect(newIndexUpdateCmd(streams)).ShouldNot(BeNil())

		o := &updateOption{
			names:     make([]string, 0),
			all:       false,
			IOStreams: streams,
		}
		Expect(o.validate([]string{defaultIndexName})).Should(Succeed())
		Expect(o.validate([]string{testIndexName})).Should(HaveOccurred())
		Expect(o.validate([]string{})).Should(HaveOccurred())
		o.all = true
		Expect(o.validate([]string{})).Should(Succeed())
		Expect(o.run()).Should(Succeed())
	})

	It("test index name", func() {
		cases := []struct {
			name    string
			success bool
		}{
			{"kubeblocks", true}, {"KubeBlocks123", true}, {"Kube_Blocks", true}, {"kube-blocks", true}, {"12345", true},
			{"kube blocks", false}, {"kube@blocks", false}, {"", false}, {"kubekubekubeblocks", false},
		}

		for _, t := range cases {
			if t.success {
				Expect(IsValidIndexName(t.name)).Should(BeTrue())
			} else {
				Expect(IsValidIndexName(t.name)).Should(BeFalse())
			}
		}
	})

	It("test get index", func() {
		indexes, err := getAllIndexes(testIndexDir)
		Expect(err).Should(Succeed())
		Expect(indexes).Should(HaveLen(1))
		Expect(indexes[0]).Should(Equal(index{
			name: defaultIndexName,
			url:  "git@github.com:apecloud/kbcli.git",
		}))
	})
})
