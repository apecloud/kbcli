/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

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

package dataprotection

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	dptypes "github.com/apecloud/kubeblocks/pkg/dataprotection/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	clientfake "k8s.io/client-go/rest/fake"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/apecloud/kubeblocks/pkg/constant"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var _ = Describe("DataProtection", func() {
	const policyName = "policy"
	const repoName = "repo"
	var streams genericiooptions.IOStreams
	var tf *cmdtesting.TestFactory
	var out *bytes.Buffer
	BeforeEach(func() {
		streams, _, out, _ = genericiooptions.NewTestIOStreams()
		tf = testing.NewTestFactory(testing.Namespace)

		_ = dpv1alpha1.AddToScheme(scheme.Scheme)
		codec := scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)
		httpResp := func(obj runtime.Object) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: cmdtesting.DefaultHeader(), Body: cmdtesting.ObjBody(codec, obj)}
		}

		tf.UnstructuredClient = &clientfake.RESTClient{
			GroupVersion:         schema.GroupVersion{Group: types.DPAPIGroup, Version: types.DPAPIVersion},
			NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
			Client: clientfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				return httpResp(testing.FakeBackup(testing.BackupName)), nil
			}),
		}

		tf.Client = tf.UnstructuredClient
	})

	AfterEach(func() {
		tf.Cleanup()
	})
	Context("backup", func() {
		initClient := func(objs ...runtime.Object) {
			clusterDef := testing.FakeClusterDef()
			cluster := testing.FakeCluster(testing.ClusterName, testing.Namespace)
			actionSet := testing.FakeActionSet()
			bpt := testing.FakeBackupPolicyTemplate()
			clusterDefLabel := map[string]string{
				constant.ClusterDefLabelKey: clusterDef.Name,
			}
			cluster.SetLabels(clusterDefLabel)
			pods := testing.FakePods(1, testing.Namespace, testing.ClusterName)
			objects := []runtime.Object{
				cluster, clusterDef, &pods.Items[0], actionSet, bpt,
			}
			objects = append(objects, objs...)
			tf.FakeDynamicClient = testing.FakeDynamicClient(objects...)
		}

		It("list-action-sets", func() {
			By("fake client")
			initClient()

			By("list action set")
			cmd := newListActionSetCmd(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			cmd.Run(cmd, nil)
			Expect(out.String()).Should(ContainSubstring(testing.ActionSetName))
			Expect(len(strings.Split(strings.Trim(out.String(), "\n"), "\n"))).Should(Equal(2))
		})

		It("list-bpt", func() {
			By("fake client")
			initClient()

			By("list bpt")
			cmd := newListBackupPolicyTemplateCmd(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			cmd.Run(cmd, nil)
			Expect(out.String()).Should(ContainSubstring(testing.BPTName))
			Expect(len(strings.Split(strings.Trim(out.String(), "\n"), "\n"))).Should(Equal(2))
		})

		It("list-backup-policies", func() {
			By("fake client")
			defaultBackupPolicy := testing.FakeBackupPolicy(policyName, testing.ClusterName)
			policy2 := testing.FakeBackupPolicy("policy1", testing.ClusterName)
			policy3 := testing.FakeBackupPolicy("policy2", testing.ClusterName)
			policy3.Namespace = "policy"
			initClient(defaultBackupPolicy, policy2, policy3)

			By("test list-backup-policies cmd")
			cmd := newListBackupPolicyCmd(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			cmd.Run(cmd, nil)
			Expect(out.String()).Should(ContainSubstring(defaultBackupPolicy.Name))
			Expect(out.String()).Should(ContainSubstring("true"))
			Expect(len(strings.Split(strings.Trim(out.String(), "\n"), "\n"))).Should(Equal(3))

			By("test list all namespace")
			out.Reset()
			_ = cmd.Flags().Set("all-namespaces", "true")
			_ = cmd.Flags().Set("cluster", testing.ClusterName)
			cmd.Run(cmd, nil)
			Expect(out.String()).Should(ContainSubstring(policy2.Name))
			Expect(len(strings.Split(strings.Trim(out.String(), "\n"), "\n"))).Should(Equal(4))
		})

		It("describe bp", func() {
			By("fake client")
			defaultBackupPolicy := testing.FakeBackupPolicy(policyName, testing.ClusterName)
			initClient(defaultBackupPolicy)

			By("describe bp")
			cmd := newDescribeBackupPolicyCmd(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			cmd.Run(cmd, []string{policyName})
			Expect(out.String()).Should(ContainSubstring(testing.BackupMethodName))
		})

		It("edit bp", func() {
			By("fake client")
			defaultBackupPolicy := testing.FakeBackupPolicy(policyName, testing.ClusterName)
			initClient(defaultBackupPolicy)

			By("edit bp")
			cmd := newEditBackupPolicyCmd(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
		})

		It("validate create backup", func() {
			By("without cluster name")
			o := &CreateBackupOptions{
				CreateOptions: action.CreateOptions{
					Dynamic:   testing.FakeDynamicClient(),
					IOStreams: streams,
					Factory:   tf,
				},
			}
			Expect(o.Validate()).To(MatchError("missing cluster name"))

			By("test without default backupPolicy")
			o.Name = testing.ClusterName
			o.Namespace = testing.Namespace
			initClient()
			o.Dynamic = tf.FakeDynamicClient
			Expect(o.Validate()).Should(MatchError(fmt.Errorf(`not found any backup policy for cluster "%s"`, testing.ClusterName)))

			By("test with two default backupPolicy")
			defaultBackupPolicy := testing.FakeBackupPolicy(policyName, testing.ClusterName)
			initClient(defaultBackupPolicy, testing.FakeBackupPolicy("policy2", testing.ClusterName))
			o.Dynamic = tf.FakeDynamicClient
			Expect(o.Validate()).Should(MatchError(fmt.Errorf(`cluster "%s" has multiple default backup policies`, o.Name)))

			By("test without method")
			initClient(defaultBackupPolicy)
			o.Dynamic = tf.FakeDynamicClient
			Expect(o.Validate().Error()).Should(ContainSubstring("backup method can not be empty, you can specify it by --method"))

			By("test without default backup repo")
			repo := testing.FakeBackupRepo(repoName, false)
			initClient(defaultBackupPolicy, repo)
			o.Dynamic = tf.FakeDynamicClient
			o.BackupSpec.BackupMethod = testing.BackupMethodName
			Expect(o.Validate()).Should(MatchError(fmt.Errorf("no default backuprepo exists")))

			By("test with two default backup repos")
			repo1 := testing.FakeBackupRepo("repo1", true)
			repo2 := testing.FakeBackupRepo("repo2", true)
			initClient(defaultBackupPolicy, repo1, repo2)
			o.Dynamic = tf.FakeDynamicClient
			o.BackupSpec.BackupMethod = testing.BackupMethodName
			Expect(o.Validate()).Should(MatchError(fmt.Errorf("cluster %s has multiple default backuprepos", o.Name)))

			By("test with one default backupPolicy")
			defaultRepo := testing.FakeBackupRepo("default-repo", true)
			initClient(defaultBackupPolicy, defaultRepo)
			o.Dynamic = tf.FakeDynamicClient
			o.BackupSpec.BackupMethod = testing.BackupMethodName
			Expect(o.Validate()).Should(Succeed())
		})

		It("run backup command", func() {
			defaultRepo := testing.FakeBackupRepo("default-repo", true)
			defaultBackupPolicy := testing.FakeBackupPolicy(policyName, testing.ClusterName)
			otherBackupPolicy := testing.FakeBackupPolicy("otherPolicy", testing.ClusterName)
			otherBackupPolicy.Annotations = map[string]string{}
			initClient(defaultBackupPolicy, otherBackupPolicy, defaultRepo)
			By("test backup with default backupPolicy")
			cmd := newBackupCommand(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			_ = cmd.Flags().Set("method", testing.BackupMethodName)
			_ = cmd.Flags().Set("cluster", testing.ClusterName)
			cmd.Run(cmd, []string{})

			By("test with specified backupMethod and backupPolicy")
			o := &CreateBackupOptions{
				CreateOptions: action.CreateOptions{
					IOStreams:       streams,
					Factory:         tf,
					GVR:             types.BackupGVR(),
					CueTemplateName: "backup_template.cue",
					Name:            testing.ClusterName,
				},
				BackupSpec: opsv1alpha1.Backup{
					BackupPolicyName: otherBackupPolicy.Name,
					BackupMethod:     testing.BackupMethodName,
				},
			}
			Expect(o.CompleteBackup()).Should(Succeed())
			err := o.Validate()
			Expect(err).Should(Succeed())
		})

		It("delete-backup", func() {
			By("expect error")
			o := action.NewDeleteOptions(tf, streams, types.BackupGVR())
			Expect(completeForDeleteBackup(o, "")).Should(HaveOccurred())
		})

		It("list-backups", func() {
			cmd := newListBackupCommand(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			By("test list-backups cmd with no backup")
			tf.FakeDynamicClient = testing.FakeDynamicClient()
			o := action.NewListOptions(tf, streams, types.BackupGVR())
			Expect(PrintBackupList(o)).Should(Succeed())
			Expect(o.ErrOut.(*bytes.Buffer).String()).Should(ContainSubstring("No backups found"))

			By("test list-backups")
			backup1 := testing.FakeBackup("test1")
			backup1.Labels = map[string]string{
				constant.AppInstanceLabelKey: "apecloud-mysql",
			}
			backup1.Status.Phase = dpv1alpha1.BackupPhaseRunning
			backup2 := testing.FakeBackup("test1")
			backup2.Namespace = "backup"
			tf.FakeDynamicClient = testing.FakeDynamicClient(backup1, backup2)
			Expect(PrintBackupList(o)).Should(Succeed())
			Expect(o.Out.(*bytes.Buffer).String()).Should(ContainSubstring("test1"))
			Expect(o.Out.(*bytes.Buffer).String()).Should(ContainSubstring("apecloud-mysql"))

			By("test list all namespace")
			o.Out.(*bytes.Buffer).Reset()
			o.AllNamespaces = true
			Expect(PrintBackupList(o)).Should(Succeed())
			Expect(len(strings.Split(strings.Trim(o.Out.(*bytes.Buffer).String(), "\n"), "\n"))).Should(Equal(3))
		})

		It("restore", func() {
			timestamp := time.Now().Format("20060102150405")
			backupName := "backup-test-" + timestamp
			clusterName := "source-cluster-" + timestamp
			newClusterName := "new-cluster-" + timestamp
			secrets := testing.FakeSecrets(testing.Namespace, clusterName)
			clusterDef := testing.FakeClusterDef()
			clusterObj := testing.FakeCluster(clusterName, testing.Namespace)
			clusterDefLabel := map[string]string{
				constant.ClusterDefLabelKey: clusterDef.Name,
			}
			clusterObj.SetLabels(clusterDefLabel)
			backupPolicy := testing.FakeBackupPolicy("backPolicy", clusterObj.Name)

			pods := testing.FakePods(1, testing.Namespace, clusterName)
			tf.FakeDynamicClient = testing.FakeDynamicClient(&secrets.Items[0],
				&pods.Items[0], clusterDef, clusterObj, backupPolicy)
			tf.Client = &clientfake.RESTClient{}
			// create backup
			backup := testing.FakeBackup(backupName)
			dynamic := testing.FakeDynamicClient(backup)
			tf.FakeDynamicClient = dynamic

			By("restore new cluster from source cluster which is not deleted")
			// mock backup is ok
			mockBackupInfo(tf.FakeDynamicClient, backupName, clusterName, nil, "")
			cmdRestore := newRestoreCommand(tf, streams)
			Expect(cmdRestore != nil).To(BeTrue())
			_ = cmdRestore.Flags().Set("cluster", newClusterName)
			cmdRestore.Run(nil, []string{backupName})
			newRestoreOps := &opsv1alpha1.OpsRequest{}
			Expect(util.GetK8SClientObject(tf.FakeDynamicClient, newRestoreOps, types.OpsGVR(), testing.Namespace, newClusterName)).Should(Succeed())
			Expect(clusterObj.Spec.ComponentSpecs[0].Replicas).Should(Equal(int32(1)))
		})

		It("describe backup", func() {
			By("fake client")
			initClient(testing.FakeBackup(testing.BackupName))

			By("describe backup")
			cmd := newBackupDescribeCommand(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			done := testing.Capture()
			cmd.Run(cmd, []string{testing.BackupName})
			capturedOutput, _ := done()
			Expect(capturedOutput).Should(ContainSubstring(testing.BackupName))
		})

		It("list-restores", func() {
			By("fake client")
			initClient(testing.FakeRestore(testing.BackupName))

			By("list restore")
			cmd := newListRestoreCommand(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			cmd.Run(cmd, nil)
			Expect(out.String()).Should(ContainSubstring(testing.RestoreName))
			Expect(len(strings.Split(strings.Trim(out.String(), "\n"), "\n"))).Should(Equal(2))
		})

		It("describe restore", func() {
			By("fake client")
			initClient(testing.FakeRestore(testing.BackupName))

			By("describe restore")
			cmd := newRestoreDescribeCommand(tf, streams)
			Expect(cmd).ShouldNot(BeNil())
			done := testing.Capture()
			cmd.Run(cmd, []string{testing.RestoreName})
			capturedOutput, _ := done()
			Expect(capturedOutput).Should(ContainSubstring(testing.RestoreName))
		})
	})
})

func mockBackupInfo(dynamic dynamic.Interface, backupName, clusterName string, timeRange map[string]any, backupMethod string) {
	clusterString := fmt.Sprintf(`{"metadata":{"name":"deleted-cluster","namespace":"%s"},"spec":{"clusterDefinitionRef":"apecloud-mysql","clusterVersionRef":"ac-mysql-8.0.30","componentSpecs":[{"name":"mysql","componentDefRef":"mysql","replicas":1}]}}`, testing.Namespace)
	backupStatus := &unstructured.Unstructured{
		Object: map[string]any{
			"status": map[string]any{
				"phase":     "Completed",
				"timeRange": timeRange,
			},
			"metadata": map[string]any{
				"name": backupName,
				"annotations": map[string]any{
					constant.ClusterSnapshotAnnotationKey:   clusterString,
					dptypes.ConnectionPasswordAnnotationKey: "test-password",
				},
				"labels": map[string]any{
					constant.AppInstanceLabelKey:    clusterName,
					constant.KBAppComponentLabelKey: "test",
				},
			},
			"spec": map[string]any{
				"backupMethod": backupMethod,
			},
		},
	}
	_, err := dynamic.Resource(types.BackupGVR()).Namespace(testing.Namespace).UpdateStatus(context.TODO(),
		backupStatus, metav1.UpdateOptions{})
	Expect(err).Should(Succeed())
}
