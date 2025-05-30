/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package cluster

import (
	"net/http"
	"os"
	"time"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	testapps "github.com/apecloud/kubeblocks/pkg/testutil/apps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
	cmdexec "k8s.io/kubectl/pkg/cmd/exec"
	cmdlogs "k8s.io/kubectl/pkg/cmd/logs"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/cluster"
	clitesting "github.com/apecloud/kbcli/pkg/testing"
)

var _ = Describe("logs", func() {
	It("isStdoutForContainer Test", func() {
		o := &LogsOptions{}
		Expect(o.isStdoutForContainer()).Should(BeTrue())
		o.fileType = "stdout"
		Expect(o.isStdoutForContainer()).Should(BeTrue())
		o.fileType = "slow"
		Expect(o.isStdoutForContainer()).Should(BeFalse())
		o.filePath = "/var/log/yum.log"
		Expect(o.isStdoutForContainer()).Should(BeFalse())
	})

	It("prefixingWriter Test", func() {
		pw := &prefixingWriter{
			prefix: []byte("prefix"),
			writer: os.Stdout,
		}
		n, _ := pw.Write([]byte(""))
		Expect(n).Should(Equal(0))
		num, _ := pw.Write([]byte("test"))
		Expect(num).Should(Equal(4))
	})

	It("assembleTailCommand Test", func() {
		command := assembleTail(true, 1, 100)
		Expect(command).ShouldNot(BeNil())
		Expect(command).Should(Equal("tail -f --lines=1 --bytes=100"))
	})

	It("addPrefixIfNeeded Test", func() {
		l := &LogsOptions{
			ExecOptions: &action.ExecOptions{
				StreamOptions: cmdexec.StreamOptions{
					ContainerName: "container",
				},
			},
		}
		// no set prefix
		w := l.addPrefixIfNeeded(corev1.ObjectReference{}, os.Stdout)
		Expect(w).Should(Equal(os.Stdout))
		// set prefix
		o := corev1.ObjectReference{
			Name:      "name",
			FieldPath: "FieldPath",
		}
		l.logOptions.Prefix = true
		w = l.addPrefixIfNeeded(o, os.Stdout)
		_, ok := w.(*prefixingWriter)
		Expect(ok).Should(BeTrue())
	})

	createTF := func() *cmdtesting.TestFactory {
		tf := cmdtesting.NewTestFactory().WithNamespace("test")
		defer tf.Cleanup()
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		ns := scheme.Codecs.WithoutConversion()
		tf.Client = &fake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: "", Version: "v1"},
			NegotiatedSerializer: ns,
			Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				body := cmdtesting.ObjBody(codec, mockPod())
				return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: body}, nil
			}),
		}
		tf.ClientConfigVal = &restclient.Config{APIPath: "/api", ContentConfig: restclient.ContentConfig{NegotiatedSerializer: scheme.Codecs, GroupVersion: &schema.GroupVersion{Version: "v1"}}}
		return tf
	}

	It("new logs command Test", func() {
		tf := createTF()
		stream := genericiooptions.NewTestIOStreamsDiscard()
		l := &LogsOptions{
			ExecOptions: action.NewExecOptions(tf, stream),
			logOptions: cmdlogs.LogsOptions{
				IOStreams: stream,
			},
		}

		cmd := NewLogsCmd(tf, stream)
		Expect(cmd).ShouldNot(BeNil())
		Expect(cmd.Use).ShouldNot(BeNil())
		Expect(cmd.Example).ShouldNot(BeNil())

		// Complete without args
		Expect(l.complete([]string{})).Should(MatchError("cluster name or instance name should be specified"))
		// Complete with args
		l.PodName = "foo"
		l.Client, _ = l.Factory.KubernetesClientSet()
		l.filePath = "/var/log"
		Expect(l.complete([]string{"cluster-name"})).Should(HaveOccurred())
		Expect(l.clusterName).Should(Equal("cluster-name"))
		// Validate stdout
		l.filePath = ""
		l.fileType = ""
		l.Namespace = "test"
		l.logOptions.SinceSeconds = time.Minute
		Expect(l.complete([]string{"cluster-name"})).Should(Succeed())
		Expect(l.validate()).Should(Succeed())
		Expect(l.logOptions.Options).ShouldNot(BeNil())

	})

	It("createFileTypeCommand Test", func() {
		tf := createTF()
		compDefName := "component-type"
		compName := "component-name"
		compDef := testapps.NewComponentDefinitionFactory(compDefName).
			Get()
		compDef.Spec.LogConfigs = []kbappsv1.LogConfig{
			{
				Name:            "slow",
				FilePathPattern: "/log/mysql/*slow.log",
			},
			{
				Name:            "error",
				FilePathPattern: "/log/mysql/*err",
			},
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "foo",
				Namespace:       "test",
				ResourceVersion: "10",
				Labels: map[string]string{
					"app.kubernetes.io/name":        "mysql-apecloud-mysql",
					constant.KBAppComponentLabelKey: compName,
				},
			},
		}
		clusterObj := &kbappsv1.Cluster{
			Spec: kbappsv1.ClusterSpec{
				ComponentSpecs: []kbappsv1.ClusterComponentSpec{
					{
						Name:         compName,
						ComponentDef: compDefName,
					},
				},
			},
		}
		tf.FakeDynamicClient = clitesting.FakeDynamicClient(compDef, pod, clusterObj)
		stream := genericiooptions.NewTestIOStreamsDiscard()
		l := &LogsOptions{
			ExecOptions: action.NewExecOptions(tf, stream),
			logOptions: cmdlogs.LogsOptions{
				IOStreams: stream,
			},
		}
		l.PodName = pod.Name
		Expect(l.Complete()).Should(Succeed())
		obj := cluster.NewClusterObjects()
		// corner case
		cmd, err := l.createFileTypeCommand(pod, obj)
		Expect(cmd).Should(Equal(""))
		Expect(err).Should(HaveOccurred())
		// normal case
		obj.Cluster = clusterObj
		l.fileType = "slow"
		cmd, err = l.createFileTypeCommand(pod, obj)
		Expect(err).Should(BeNil())
		Expect(cmd).Should(Equal("ls /log/mysql/*slow.log | xargs tail --lines=0"))
		// error case
		l.fileType = "slow-error"
		cmd, err = l.createFileTypeCommand(pod, obj)
		Expect(err).Should(HaveOccurred())
		Expect(cmd).Should(Equal(""))
	})
})

func mockPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "foo",
			Namespace:       "test",
			ResourceVersion: "10",
			Labels: map[string]string{
				"app.kubernetes.io/name": "mysql-apecloud-mysql",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			DNSPolicy:     corev1.DNSClusterFirst,
			Containers: []corev1.Container{
				{
					Name: "bar",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}
