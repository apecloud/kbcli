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

package util

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	mrand "math/rand"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	opsv1alpha1 "github.com/apecloud/kubeblocks/apis/operations/v1alpha1"
	parametersv1alpha1 "github.com/apecloud/kubeblocks/apis/parameters/v1alpha1"
	intctrlutil "github.com/apecloud/kubeblocks/pkg/controllerutil"
	"github.com/apecloud/kubeblocks/pkg/generics"
	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sapitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	cmdget "k8s.io/kubectl/pkg/cmd/get"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	kbappsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	kbappsv1beta1 "github.com/apecloud/kubeblocks/apis/apps/v1beta1"
	"github.com/apecloud/kubeblocks/pkg/configuration/core"
	"github.com/apecloud/kubeblocks/pkg/configuration/openapi"
	cfgutil "github.com/apecloud/kubeblocks/pkg/configuration/util"
	"github.com/apecloud/kubeblocks/pkg/constant"
	viper "github.com/apecloud/kubeblocks/pkg/viperx"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

// CloseQuietly closes `io.Closer` quietly. Very handy and helpful for code
// quality too.
func CloseQuietly(d io.Closer) {
	_ = d.Close()
}

// GetCliHomeDir returns kbcli home dir
func GetCliHomeDir() (string, error) {
	var cliHome string
	if custom := os.Getenv(types.CliHomeEnv); custom != "" {
		cliHome = custom
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cliHome = filepath.Join(home, types.CliDefaultHome)
	}
	if _, err := os.Stat(cliHome); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(cliHome, 0750); err != nil {
			return "", errors.Wrap(err, "error when create kbcli home directory")
		}
	}
	return cliHome, nil
}

// GetCliLogDir returns kbcli log dir
func GetCliLogDir() (string, error) {
	cliHome, err := GetCliHomeDir()
	if err != nil {
		return "", err
	}
	logDir := filepath.Join(cliHome, types.CliLogDir)
	if _, err := os.Stat(logDir); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(logDir, 0750); err != nil {
			return "", errors.Wrap(err, "error when create kbcli log directory")
		}
	}
	return logDir, nil
}

// GetCliAddonDir returns kbcli addon index dir
func GetCliAddonDir() (string, error) {
	var addonIndexDir string
	if custom := os.Getenv(types.AddonIndexDirEnv); custom != "" {
		addonIndexDir = custom
	} else {
		home, err := GetCliHomeDir()
		if err != nil {
			return "", err
		}
		addonIndexDir = path.Join(home, types.AddonIndexDir)
	}

	if _, err := os.Stat(addonIndexDir); err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(addonIndexDir, 0750); err != nil {
			return "", errors.Wrap(err, "error when create addon index directory")
		}
	}

	return addonIndexDir, nil
}

// GetKubeconfigDir returns the kubeconfig directory.
func GetKubeconfigDir() string {
	var kubeconfigDir string
	switch runtime.GOOS {
	case types.GoosDarwin, types.GoosLinux:
		kubeconfigDir = filepath.Join(os.Getenv("HOME"), ".kube")
	case types.GoosWindows:
		kubeconfigDir = filepath.Join(os.Getenv("USERPROFILE"), ".kube")
	}
	return kubeconfigDir
}

func ConfigPath(name string) string {
	if len(name) == 0 {
		return ""
	}

	return filepath.Join(GetKubeconfigDir(), name)
}

func RemoveConfig(name string) error {
	if err := os.Remove(ConfigPath(name)); err != nil {
		return err
	}
	return nil
}

func GetPublicIP() (string, error) {
	resp, err := http.Get("https://ifconfig.me")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// MakeSSHKeyPair makes a pair of public and private keys for SSH access.
// Public key is encoded in the format for inclusion in an OpenSSH authorized_keys file.
// Private Key generated is PEM encoded
func MakeSSHKeyPair(pubKeyPath, privateKeyPath string) error {
	if err := os.MkdirAll(path.Dir(pubKeyPath), os.FileMode(0700)); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(privateKeyPath), os.FileMode(0700)); err != nil {
		return err
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	// generate and write private key as PEM
	privateKeyFile, err := os.OpenFile(privateKeyPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer privateKeyFile.Close()

	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}
	return os.WriteFile(pubKeyPath, ssh.MarshalAuthorizedKey(pub), 0655)
}

func PrintObjYAML(obj *unstructured.Unstructured) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type RetryOptions struct {
	MaxRetry int
	Delay    time.Duration
}

func DoWithRetry(ctx context.Context, logger logr.Logger, operation func() error, options *RetryOptions) error {
	err := operation()
	for attempt := 0; err != nil && attempt < options.MaxRetry; attempt++ {
		delay := time.Duration(int(math.Pow(2, float64(attempt)))) * time.Second
		if options.Delay != 0 {
			delay = options.Delay
		}
		logger.Info(fmt.Sprintf("Failed, retrying in %s ... (%d/%d). Error: %v", delay, attempt+1, options.MaxRetry, err))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return err
		}
		err = operation()
	}
	return err
}

func PrintGoTemplate(wr io.Writer, tpl string, values interface{}) error {
	tmpl, err := template.New("output").Parse(tpl)
	if err != nil {
		return err
	}

	err = tmpl.Execute(wr, values)
	if err != nil {
		return err
	}
	return nil
}

// SetKubeConfig sets KUBECONFIG environment
func SetKubeConfig(cfg string) error {
	return os.Setenv("KUBECONFIG", cfg)
}

var addToScheme sync.Once

func NewFactory() cmdutil.Factory {
	configFlags := NewConfigFlagNoWarnings()
	// Add CRDs to the scheme. They are missing by default.
	addToScheme.Do(func() {
		if err := apiextv1.AddToScheme(scheme.Scheme); err != nil {
			// This should never happen.
			panic(err)
		}
	})
	return cmdutil.NewFactory(configFlags)
}

// NewConfigFlagNoWarnings returns a ConfigFlags that disables warnings.
func NewConfigFlagNoWarnings() *genericclioptions.ConfigFlags {
	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
		c.WarningHandler = rest.NoWarnings{}
		return c
	}
	return configFlags
}

func GVRToString(gvr schema.GroupVersionResource) string {
	return strings.Join([]string{gvr.Resource, gvr.Version, gvr.Group}, ".")
}

// GetNodeByName chooses node by name from a node array
func GetNodeByName(nodes []*corev1.Node, name string) *corev1.Node {
	for _, node := range nodes {
		if node.Name == name {
			return node
		}
	}
	return nil
}

// ResourceIsEmpty checks if resource is empty or not
func ResourceIsEmpty(res *resource.Quantity) bool {
	resStr := res.String()
	if resStr == "0" || resStr == "<nil>" {
		return true
	}
	return false
}

func GetPodStatus(pods []corev1.Pod) (running, waiting, succeeded, failed int) {
	for _, pod := range pods {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			running++
		case corev1.PodPending:
			waiting++
		case corev1.PodSucceeded:
			succeeded++
		case corev1.PodFailed:
			failed++
		}
	}
	return
}

// OpenBrowser opens browser with url in different OS system
func OpenBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("cmd", "/C", "start", url).Run()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

func TimeFormat(t *metav1.Time) string {
	return TimeFormatWithDuration(t, time.Minute)
}

// TimeFormatWithDuration formats time with specified precision
func TimeFormatWithDuration(t *metav1.Time, duration time.Duration) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return TimeTimeFormatWithDuration(t.Time, duration)
}

func TimeTimeFormat(t time.Time) string {
	const layout = "Jan 02,2006 15:04 UTC-0700"
	return t.Format(layout)
}

func timeLayout(precision time.Duration) string {
	layout := "Jan 02,2006 15:04 UTC-0700"
	switch precision {
	case time.Second:
		layout = "Jan 02,2006 15:04:05 UTC-0700"
	case time.Millisecond:
		layout = "Jan 02,2006 15:04:05.000 UTC-0700"
	}
	return layout
}

func TimeTimeFormatWithDuration(t time.Time, precision time.Duration) string {
	layout := timeLayout(precision)
	return t.Format(layout)
}

func TimeParse(t string, precision time.Duration) (time.Time, error) {
	layout := timeLayout(precision)
	return time.Parse(layout, t)
}

// GetHumanReadableDuration returns a succinct representation of the provided startTime and endTime
// with limited precision for consumption by humans.
func GetHumanReadableDuration(startTime metav1.Time, endTime metav1.Time) string {
	if startTime.IsZero() {
		return "<Unknown>"
	}
	if endTime.IsZero() {
		endTime = metav1.NewTime(time.Now())
	}
	d := endTime.Sub(startTime.Time)
	// if the
	if d < time.Second {
		d = time.Second
	}
	return duration.HumanDuration(d)
}

// CheckEmpty checks if string is empty, if yes, returns <none> for displaying
func CheckEmpty(str string) string {
	if len(str) == 0 {
		return types.None
	}
	return str
}

// BuildLabelSelectorByNames builds the label selector by instance names, the label selector is
// like "instance-key in (name1, name2)"
func BuildLabelSelectorByNames(selector string, names []string) string {
	if len(names) == 0 {
		return selector
	}

	label := fmt.Sprintf("%s in (%s)", constant.AppInstanceLabelKey, strings.Join(names, ","))
	if len(selector) == 0 {
		return label
	} else {
		return selector + "," + label
	}
}

// SortEventsByLastTimestamp sorts events by lastTimestamp
func SortEventsByLastTimestamp(events *corev1.EventList, eventType string) *[]apiruntime.Object {
	objs := make([]apiruntime.Object, 0, len(events.Items))
	for i, e := range events.Items {
		if eventType != "" && e.Type != eventType {
			continue
		}
		objs = append(objs, &events.Items[i])
	}
	sorter := cmdget.NewRuntimeSort("{.lastTimestamp}", objs)
	sort.Sort(sorter)
	return &objs
}

func GetEventTimeStr(e *corev1.Event) string {
	t := &e.CreationTimestamp
	if !e.LastTimestamp.Time.IsZero() {
		t = &e.LastTimestamp
	}
	return TimeFormat(t)
}

func GetEventObject(e *corev1.Event) string {
	kind := e.InvolvedObject.Kind
	if kind == "Pod" {
		kind = "Instance"
	}
	return fmt.Sprintf("%s/%s", kind, e.InvolvedObject.Name)
}

// ComponentConfigSpecs returns configSpecs used by the component.
// func ComponentConfigSpecs(clusterName string, namespace string, cli dynamic.Interface, componentName string, reloadTpl bool) ([]kbappsv1alpha1.ComponentConfigSpec, error) {
// 	var (
// 		clusterObj    = kbappsv1.Cluster{}
// 		clusterDefObj = kbappsv1.ClusterDefinition{}
// 	)
//
// 	clusterKey := client.ObjectKey{
// 		Namespace: namespace,
// 		Name:      clusterName,
// 	}
// 	if err := GetResourceObjectFromGVR(types.ClusterGVR(), clusterKey, cli, &clusterObj); err != nil {
// 		return nil, err
// 	}
// 	clusterDefKey := client.ObjectKey{
// 		Namespace: "",
// 		Name:      clusterObj.Spec.ClusterDef,
// 	}
// 	if err := GetResourceObjectFromGVR(types.ClusterDefGVR(), clusterDefKey, cli, &clusterDefObj); err != nil {
// 		return nil, err
// 	}
// 	compDef, err := GetComponentDefByCompName(cli, &clusterObj, componentName)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return GetValidConfigSpecs(reloadTpl, ToV1ComponentConfigSpecs(compDef.Spec.Configs))
// }

func GetComponentDefByName(dynamic dynamic.Interface, name string) (*kbappsv1.ComponentDefinition, error) {
	componentDef := &kbappsv1.ComponentDefinition{}
	if err := GetK8SClientObject(dynamic, componentDef, types.CompDefGVR(), "", name); err != nil {
		return nil, err
	}
	return componentDef, nil
}

// GetComponentDefByCompName gets the ComponentDefinition object by the component name.
func GetComponentDefByCompName(cli dynamic.Interface, clusterObj *kbappsv1.Cluster, compName string) (*kbappsv1.ComponentDefinition, error) {
	var compDefName string
	compSpec := clusterObj.Spec.GetComponentByName(compName)
	if compSpec != nil {
		compDefName = compSpec.ComponentDef
	} else {
		shardingSpec := clusterObj.Spec.GetShardingByName(compName)
		if shardingSpec != nil {
			compDefName = shardingSpec.Template.ComponentDef
		}
	}
	return GetComponentDefByName(cli, compDefName)
}

func GetValidConfigSpecs(reloadTpl bool, configSpecs []kbappsv1alpha1.ComponentConfigSpec) ([]kbappsv1alpha1.ComponentConfigSpec, error) {
	if !reloadTpl || len(configSpecs) == 1 {
		return configSpecs, nil
	}

	validConfigSpecs := make([]kbappsv1alpha1.ComponentConfigSpec, 0, len(configSpecs))
	for _, configSpec := range configSpecs {
		if configSpec.ConfigConstraintRef != "" && configSpec.TemplateRef != "" {
			validConfigSpecs = append(validConfigSpecs, configSpec)
		}
	}
	return validConfigSpecs, nil
}

func GetConfigSpecsFromComponentName(cli dynamic.Interface, namespace, clusterName, componentName string, reloadTpl bool) ([]kbappsv1alpha1.ComponentConfigSpec, error) {
	configKey := client.ObjectKey{
		Namespace: namespace,
		Name:      core.GenerateComponentConfigurationName(clusterName, componentName),
	}
	config := kbappsv1alpha1.Configuration{}
	if err := GetResourceObjectFromGVR(types.ConfigurationGVR(), configKey, cli, &config); err != nil {
		return nil, err
	}
	if len(config.Spec.ConfigItemDetails) == 0 {
		return nil, nil
	}

	configSpecs := make([]kbappsv1alpha1.ComponentConfigSpec, 0, len(config.Spec.ConfigItemDetails))
	for _, item := range config.Spec.ConfigItemDetails {
		if item.ConfigSpec != nil {
			configSpecs = append(configSpecs, *item.ConfigSpec)
		}
	}
	return GetValidConfigSpecs(reloadTpl, configSpecs)
}

// func ToV1ComponentConfigSpec(configSpec kbappsv1.ComponentConfigSpec) kbappsv1alpha1.ComponentConfigSpec {
// 	config := kbappsv1alpha1.ComponentConfigSpec{
// 		ComponentTemplateSpec: kbappsv1alpha1.ComponentTemplateSpec{
// 			Name:        configSpec.Name,
// 			TemplateRef: configSpec.TemplateRef,
// 			Namespace:   configSpec.Namespace,
// 			VolumeName:  configSpec.VolumeName,
// 			DefaultMode: configSpec.DefaultMode,
// 		},
// 		Keys:                configSpec.Keys,
// 		ConfigConstraintRef: configSpec.ConfigConstraintRef,
// 		InjectEnvTo:         configSpec.InjectEnvTo,
// 		AsSecret:            configSpec.AsSecret,
// 	}
// 	for i := range configSpec.ReRenderResourceTypes {
// 		config.ReRenderResourceTypes = append(config.ReRenderResourceTypes, kbappsv1alpha1.RerenderResourceType(configSpec.ReRenderResourceTypes[i]))
// 	}
// 	return config
// }
//
// func ToV1ComponentConfigSpecs(configSpecs []kbappsv1.ComponentConfigSpec) []kbappsv1alpha1.ComponentConfigSpec {
// 	var configs []kbappsv1alpha1.ComponentConfigSpec
// 	for i := range configSpecs {
// 		configs = append(configs, ToV1ComponentConfigSpec(configSpecs[i]))
// 	}
// 	return configs
// }

// GetK8SClientObject gets the client object of k8s,
// obj must be a struct pointer so that obj can be updated with the response.
func GetK8SClientObject(dynamic dynamic.Interface,
	obj client.Object,
	gvr schema.GroupVersionResource,
	namespace,
	name string) error {
	unstructuredObj, err := dynamic.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.UnstructuredContent(), obj)
}

// GetResourceObjectFromGVR queries the resource object using GVR.
func GetResourceObjectFromGVR(gvr schema.GroupVersionResource, key client.ObjectKey, client dynamic.Interface, k8sObj interface{}) error {
	unstructuredObj, err := client.
		Resource(gvr).
		Namespace(key.Namespace).
		Get(context.TODO(), key.Name, metav1.GetOptions{})
	if err != nil {
		return core.WrapError(err, "failed to get resource[%v]", key)
	}
	return apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, k8sObj)
}

// GetComponentsFromResource returns name of component.
func GetComponentsFromResource(namespace, clusterName string, componentSpecs []kbappsv1.ClusterComponentSpec, cli dynamic.Interface) ([]string, error) {
	componentNames := make([]string, 0, len(componentSpecs))
	for _, component := range componentSpecs {
		configKey := client.ObjectKey{
			Namespace: namespace,
			Name:      core.GenerateComponentConfigurationName(clusterName, component.Name),
		}
		config := kbappsv1alpha1.Configuration{}
		if err := GetResourceObjectFromGVR(types.ConfigurationGVR(), configKey, cli, &config); err != nil {
			return nil, err
		}
		if len(config.Spec.ConfigItemDetails) == 0 {
			continue
		}

		if enableReconfiguring(&config.Spec) {
			componentNames = append(componentNames, component.Name)
		}
	}
	return componentNames, nil
}

func IsSupportConfigFileReconfigure(configTemplateSpec kbappsv1alpha1.ComponentConfigSpec, configFileKey string) bool {
	if len(configTemplateSpec.Keys) == 0 {
		return true
	}
	for _, keySelector := range configTemplateSpec.Keys {
		if keySelector == configFileKey {
			return true
		}
	}
	return false
}

func enableReconfiguring(component *kbappsv1alpha1.ConfigurationSpec) bool {
	if component == nil {
		return false
	}
	for _, item := range component.ConfigItemDetails {
		if item.ConfigSpec == nil {
			continue
		}
		tpl := item.ConfigSpec
		if len(tpl.ConfigConstraintRef) > 0 && len(tpl.TemplateRef) > 0 {
			return true
		}
	}
	return false
}

// IsSupportReconfigureParams checks whether all updated parameters belong to config template parameters.
func IsSupportReconfigureParams(tpl kbappsv1alpha1.ComponentConfigSpec, values map[string]*string, cli dynamic.Interface) (bool, error) {
	var (
		err              error
		configConstraint = kbappsv1beta1.ConfigConstraint{}
	)

	if err := GetResourceObjectFromGVR(types.ConfigConstraintGVR(), client.ObjectKey{
		Namespace: "",
		Name:      tpl.ConfigConstraintRef,
	}, cli, &configConstraint); err != nil {
		return false, err
	}

	if configConstraint.Spec.ParametersSchema == nil {
		return true, nil
	}

	schema := configConstraint.Spec.ParametersSchema.DeepCopy()
	if schema.SchemaInJSON == nil {
		schema.SchemaInJSON, err = openapi.GenerateOpenAPISchema(schema.CUE, schema.TopLevelKey)
		if err != nil {
			return false, err
		}
		if schema.SchemaInJSON == nil {
			return true, nil
		}
	}

	schemaSpec := schema.SchemaInJSON.Properties["spec"]
	for key := range values {
		if _, ok := schemaSpec.Properties[key]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func ValidateParametersModified(classifyParameters map[string]map[string]*parametersv1alpha1.ParametersInFile, pds []*parametersv1alpha1.ParametersDefinition) (err error) {
	validator := func(index int, parameters sets.Set[string]) error {
		if index < 0 || len(pds[index].Spec.ImmutableParameters) == 0 {
			return nil
		}
		immuSet := sets.New(pds[index].Spec.ImmutableParameters...)
		uniqueParameters := immuSet.Intersection(parameters)
		if uniqueParameters.Len() == 0 {
			return nil
		}
		return core.MakeError("parameter[%v] is immutable, cannot be modified!", cfgutil.ToSet(uniqueParameters).AsSlice())
	}

	for _, tplParams := range classifyParameters {
		for file, params := range tplParams {
			match := func(pd *parametersv1alpha1.ParametersDefinition) bool {
				return pd.Spec.FileName == file
			}
			index := generics.FindFirstFunc(pds, match)
			if err := validator(index, sets.KeySet(params.Parameters)); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateParametersModified2(parameters sets.Set[string], pds []*parametersv1alpha1.ParametersDefinition, file string) error {
	var ret *parametersv1alpha1.ParametersDefinition
	for _, pd := range pds {
		if pd.Spec.FileName == file {
			ret = pd
			break
		}
	}

	if ret == nil || len(ret.Spec.ImmutableParameters) == 0 {
		return nil
	}

	immutableParameters := sets.New(ret.Spec.ImmutableParameters...)
	uniqueParameters := immutableParameters.Intersection(parameters)
	if uniqueParameters.Len() == 0 {
		return nil
	}
	return core.MakeError("parameter[%v] is immutable, cannot be modified!", cfgutil.ToSet(uniqueParameters).AsSlice())
}

func GetIPLocation() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://ifconfig.io/country_code", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	location, err := io.ReadAll(resp.Body)
	if len(location) == 0 || err != nil {
		return "", err
	}

	// remove last "\n"
	return string(location[:len(location)-1]), nil
}

// GetHelmChartRepoURL gets helm chart repo, chooses one from GitHub and GitLab based on the IP location
func GetHelmChartRepoURL() string {
	if types.KubeBlocksChartURL == testing.KubeBlocksChartURL {
		return testing.KubeBlocksChartURL
	}

	// if helm repo url is specified by config or environment, use it
	url := viper.GetString(types.CfgKeyHelmRepoURL)
	if url != "" {
		klog.V(1).Infof("Using helm repo url set by config or environment: %s", url)
		return url
	}

	// if helm repo url is not specified, choose one from GitHub and GitLab based on the IP location
	// if location is CN, or we can not get location, use GitLab helm chart repo
	repo := types.KubeBlocksChartURL
	location, _ := GetIPLocation()
	if location == "CN" || location == "" {
		repo = types.GitLabHelmChartRepo
	}
	klog.V(1).Infof("Using helm repo url: %s", repo)
	return repo
}

// GetKubeBlocksNamespace gets namespace of KubeBlocks installation, infer namespace from helm secrets
func GetKubeBlocksNamespace(client kubernetes.Interface, specifiedNamespace string) (string, error) {
	secrets, err := client.CoreV1().Secrets(specifiedNamespace).List(context.TODO(), metav1.ListOptions{LabelSelector: types.KubeBlocksHelmLabel})
	if err != nil {
		return "", err
	}
	var kbNamespace string
	for _, v := range secrets.Items {
		if kbNamespace == "" {
			kbNamespace = v.Namespace
		}
		if kbNamespace != v.Namespace {
			return kbNamespace, intctrlutil.NewFatalError(fmt.Sprintf(`Existing multiple KubeBlocks installation namespace: "%s, %s", need to manually specify the namespace flag`, kbNamespace, v.Namespace))
		}
	}
	// if KubeBlocks is upgraded, there will be multiple secrets
	if kbNamespace == "" {
		return kbNamespace, errors.New("failed to get KubeBlocks installation namespace")
	}
	return kbNamespace, nil
}

// GetKubeBlocksCRDsURL gets the crds url by IP region.
func GetKubeBlocksCRDsURL(kbVersion string) string {
	kbVersion = TrimVersionPrefix(kbVersion)
	location, _ := GetIPLocation()
	crdsURL := fmt.Sprintf("https://github.com/apecloud/kubeblocks/releases/download/v%s/kubeblocks_crds.yaml", kbVersion)
	if location == "CN" || location == "" {
		crdsURL = fmt.Sprintf("https://jihulab.com/api/v4/projects/98723/packages/generic/kubeblocks/v%s/kubeblocks_crds.yaml", kbVersion)
	}
	klog.V(1).Infof("CRDs url: %s", crdsURL)
	return crdsURL
}

// GetKubeBlocksNamespaceByDynamic gets namespace of KubeBlocks installation, infer namespace from helm secrets
func GetKubeBlocksNamespaceByDynamic(dynamic dynamic.Interface) (string, error) {
	list, err := dynamic.Resource(types.SecretGVR()).List(context.TODO(), metav1.ListOptions{LabelSelector: types.KubeBlocksHelmLabel})
	if err == nil && len(list.Items) >= 1 {
		return list.Items[0].GetNamespace(), nil
	}
	return "", errors.New("failed to get KubeBlocks installation namespace")
}

type ExposeType string

const (
	ExposeToIntranet ExposeType = "intranet"
	ExposeToInternet ExposeType = "internet"

	NodePort     string = "NodePort"
	LoadBalancer string = "LoadBalancer"

	EnableValue  string = "true"
	DisableValue string = "false"
)

var ProviderExposeAnnotations = map[K8sProvider]map[ExposeType]map[string]string{
	EKSProvider: {
		ExposeToIntranet: map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type":     "nlb",
			"service.beta.kubernetes.io/aws-load-balancer-internal": "true",
		},
		ExposeToInternet: map[string]string{
			"service.beta.kubernetes.io/aws-load-balancer-type":     "nlb",
			"service.beta.kubernetes.io/aws-load-balancer-internal": "false",
		},
	},
	GKEProvider: {
		ExposeToIntranet: map[string]string{
			"networking.gke.io/load-balancer-type": "Internal",
		},
		ExposeToInternet: map[string]string{},
	},
	AKSProvider: {
		ExposeToIntranet: map[string]string{
			"service.beta.kubernetes.io/azure-load-balancer-internal": "true",
		},
		ExposeToInternet: map[string]string{
			"service.beta.kubernetes.io/azure-load-balancer-internal": "false",
		},
	},
	ACKProvider: {
		ExposeToIntranet: map[string]string{
			"service.beta.kubernetes.io/alibaba-cloud-loadbalancer-address-type": "intranet",
		},
		ExposeToInternet: map[string]string{
			"service.beta.kubernetes.io/alibaba-cloud-loadbalancer-address-type": "internet",
		},
	},
	// TKE VPC LoadBalancer needs the subnet id, it's difficult for KB to get it, so we just support the internet on TKE now.
	// reference: https://cloud.tencent.com/document/product/457/45487
	TKEProvider: {
		ExposeToInternet: map[string]string{},
	},
	KINDProvider: {
		ExposeToIntranet: map[string]string{},
	},
	K3SProvider: {
		ExposeToIntranet: map[string]string{},
	},
	UnknownProvider: {
		ExposeToIntranet: map[string]string{},
		ExposeToInternet: map[string]string{},
	},
}

func GetExposeAnnotations(provider K8sProvider, exposeType ExposeType) (map[string]string, error) {
	exposeAnnotations, ok := ProviderExposeAnnotations[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	annotations, ok := exposeAnnotations[exposeType]
	if !ok {
		return nil, fmt.Errorf("unsupported expose type: %s on provider %s", exposeType, provider)
	}
	return annotations, nil
}

// BuildAddonReleaseName returns the release name of addon, its f
func BuildAddonReleaseName(addon string) string {
	return fmt.Sprintf("%s-%s", types.AddonReleasePrefix, addon)
}

// CombineLabels combines labels into a string
func CombineLabels(labels map[string]string) string {
	var labelStr []string
	for k, v := range labels {
		labelStr = append(labelStr, fmt.Sprintf("%s=%s", k, v))
	}

	// sort labelStr to make sure the order is stable
	sort.Strings(labelStr)

	return strings.Join(labelStr, ",")
}

func BuildComponentNameLabels(prefix string, names []string) string {
	return buildLabelSelectors(prefix, constant.KBAppComponentLabelKey, names)
}

// buildLabelSelectors builds the label selector by given label key, the label selector is
// like "label-key in (name1, name2)"
func buildLabelSelectors(prefix string, key string, names []string) string {
	if len(names) == 0 {
		return prefix
	}

	label := fmt.Sprintf("%s in (%s)", key, strings.Join(names, ","))
	if len(prefix) == 0 {
		return label
	} else {
		return prefix + "," + label
	}
}

// NewOpsRequestForReconfiguring returns a new common OpsRequest for Reconfiguring operation
func NewOpsRequestForReconfiguring(opsName, namespace, clusterName string) *opsv1alpha1.OpsRequest {
	return &opsv1alpha1.OpsRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fmt.Sprintf("%s/%s", types.AppsAPIGroup, types.AppsAPIVersion),
			Kind:       types.KindOps,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      opsName,
			Namespace: namespace,
		},
		Spec: opsv1alpha1.OpsRequestSpec{
			ClusterName: clusterName,
			Type:        opsv1alpha1.ReconfiguringType,
			SpecificOpsRequest: opsv1alpha1.SpecificOpsRequest{
				Reconfigures: []opsv1alpha1.Reconfigure{},
			},
		},
	}
}
func ConvertObjToUnstructured(obj any) (*unstructured.Unstructured, error) {
	var (
		contentBytes    []byte
		err             error
		unstructuredObj = &unstructured.Unstructured{}
	)

	if contentBytes, err = json.Marshal(obj); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(contentBytes, unstructuredObj); err != nil {
		return nil, err
	}
	return unstructuredObj, nil
}

func CreateResourceIfAbsent(
	dynamic dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace string,
	unstructuredObj *unstructured.Unstructured) error {
	objectName, isFound, err := unstructured.NestedString(unstructuredObj.Object, "metadata", "name")
	if !isFound || err != nil {
		return err
	}
	objectByte, err := json.Marshal(unstructuredObj)
	if err != nil {
		return err
	}
	if _, err = dynamic.Resource(gvr).Namespace(namespace).Patch(
		context.TODO(), objectName, k8sapitypes.MergePatchType,
		objectByte, metav1.PatchOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err = dynamic.Resource(gvr).Namespace(namespace).Create(
				context.TODO(), unstructuredObj, metav1.CreateOptions{}); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func BuildClusterDefinitionRefLabel(prefix string, clusterDef []string) string {
	return buildLabelSelectors(prefix, constant.AppNameLabelKey, clusterDef)
}

func BuildClusterLabel(prefix string, addon []string) string {
	return buildLabelSelectors(prefix, constant.ClusterDefLabelKey, addon)
}

// IsWindows returns true if the kbcli runtime situation is windows
func IsWindows() bool {
	return runtime.GOOS == types.GoosWindows
}

func GetUnifiedDiffString(original, edited string, from, to string, contextLine int) (string, error) {
	if contextLine <= 0 {
		contextLine = 3
	}
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(edited),
		FromFile: from,
		ToFile:   to,
		Context:  contextLine,
	}
	return difflib.GetUnifiedDiffString(diff)
}

func DisplayDiffWithColor(out io.Writer, diffText string) {
	for _, line := range difflib.SplitLines(diffText) {
		switch {
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			line = color.HiYellowString(line)
		case strings.HasPrefix(line, "@@"):
			line = color.HiBlueString(line)
		case strings.HasPrefix(line, "-"):
			line = color.RedString(line)
		case strings.HasPrefix(line, "+"):
			line = color.GreenString(line)
		}
		fmt.Fprint(out, line)
	}
}

func BuildSchedulingPolicy(tenancy string, clusterName string, compName string, tolerations []corev1.Toleration, nodeLabels map[string]string, podAntiAffinity string, topologyKeys []string) (*kbappsv1.SchedulingPolicy, bool) {
	if len(tolerations) == 0 && len(nodeLabels) == 0 && len(topologyKeys) == 0 {
		return nil, false
	}
	affinity := &corev1.Affinity{}
	if podAntiAffinity != "" || len(topologyKeys) > 0 {
		affinity.PodAntiAffinity = BuildPodAntiAffinityForComponent(tenancy, clusterName, compName, podAntiAffinity, topologyKeys)
	}

	var topologySpreadConstraints []corev1.TopologySpreadConstraint

	var whenUnsatisfiable corev1.UnsatisfiableConstraintAction
	if kbappsv1alpha1.PodAntiAffinity(podAntiAffinity) == kbappsv1alpha1.Required {
		whenUnsatisfiable = corev1.DoNotSchedule
	} else {
		whenUnsatisfiable = corev1.ScheduleAnyway
	}
	for _, topologyKey := range topologyKeys {
		topologySpreadConstraints = append(topologySpreadConstraints, corev1.TopologySpreadConstraint{
			MaxSkew:           1,
			WhenUnsatisfiable: whenUnsatisfiable,
			TopologyKey:       topologyKey,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constant.AppInstanceLabelKey:    clusterName,
					constant.KBAppComponentLabelKey: compName,
				},
			},
		})
	}

	schedulingPolicy := &kbappsv1.SchedulingPolicy{
		Affinity:                  affinity,
		NodeSelector:              nodeLabels,
		Tolerations:               tolerations,
		TopologySpreadConstraints: topologySpreadConstraints,
	}

	return schedulingPolicy, true
}

// BuildTolerations toleration format: key=value:effect or key:effect,
func BuildTolerations(raw []string) ([]interface{}, error) {
	tolerations := make([]interface{}, 0)
	for _, tolerationRaw := range raw {
		for _, entries := range strings.Split(tolerationRaw, ",") {
			toleration := make(map[string]interface{})
			parts := strings.Split(entries, ":")
			if len(parts) != 2 {
				return tolerations, fmt.Errorf("invalid toleration %s", entries)
			}
			toleration["effect"] = parts[1]

			partsKV := strings.Split(parts[0], "=")
			switch len(partsKV) {
			case 1:
				toleration["operator"] = "Exists"
				toleration["key"] = partsKV[0]
			case 2:
				toleration["operator"] = "Equal"
				toleration["key"] = partsKV[0]
				toleration["value"] = partsKV[1]
			default:
				return tolerations, fmt.Errorf("invalid toleration %s", entries)
			}
			tolerations = append(tolerations, toleration)
		}
	}
	return tolerations, nil
}

// BuildNodeAffinity build node affinity from node labels
func BuildNodeAffinity(nodeLabels map[string]string) *corev1.NodeAffinity {
	var nodeAffinity *corev1.NodeAffinity

	var matchExpressions []corev1.NodeSelectorRequirement
	for key, value := range nodeLabels {
		values := strings.Split(value, ",")
		matchExpressions = append(matchExpressions, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   values,
		})
	}
	if len(matchExpressions) > 0 {
		nodeSelectorTerm := corev1.NodeSelectorTerm{
			MatchExpressions: matchExpressions,
		}
		nodeAffinity = &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Preference: nodeSelectorTerm,
				},
			},
		}
	}

	return nodeAffinity
}

// BuildPodAntiAffinity build pod anti affinity from topology keys
func BuildPodAntiAffinity(podAntiAffinityStrategy string, topologyKeys []string) *corev1.PodAntiAffinity {
	var podAntiAffinity *corev1.PodAntiAffinity
	var podAffinityTerms []corev1.PodAffinityTerm
	for _, topologyKey := range topologyKeys {
		podAffinityTerms = append(podAffinityTerms, corev1.PodAffinityTerm{
			TopologyKey: topologyKey,
		})
	}
	if podAntiAffinityStrategy == string(kbappsv1alpha1.Required) {
		podAntiAffinity = &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: podAffinityTerms,
		}
	} else {
		var weightedPodAffinityTerms []corev1.WeightedPodAffinityTerm
		for _, podAffinityTerm := range podAffinityTerms {
			weightedPodAffinityTerms = append(weightedPodAffinityTerms, corev1.WeightedPodAffinityTerm{
				Weight:          100,
				PodAffinityTerm: podAffinityTerm,
			})
		}
		podAntiAffinity = &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: weightedPodAffinityTerms,
		}
	}

	return podAntiAffinity
}

// BuildPodAntiAffinityForComponent build pod anti affinity from topology keys and tenancy for cluster
func BuildPodAntiAffinityForComponent(tenancy string, clusterName string, compName string, podAntiAffinityStrategy string, topologyKeys []string) *corev1.PodAntiAffinity {
	var podAntiAffinity *corev1.PodAntiAffinity
	var podAffinityTerms []corev1.PodAffinityTerm
	for _, topologyKey := range topologyKeys {
		podAffinityTerms = append(podAffinityTerms, corev1.PodAffinityTerm{
			TopologyKey: topologyKey,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constant.AppInstanceLabelKey:    clusterName,
					constant.KBAppComponentLabelKey: compName,
				},
			},
		})
	}
	if podAntiAffinityStrategy == string(kbappsv1alpha1.Required) {
		podAntiAffinity = &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: podAffinityTerms,
		}
	} else {
		var weightedPodAffinityTerms []corev1.WeightedPodAffinityTerm
		for _, podAffinityTerm := range podAffinityTerms {
			weightedPodAffinityTerms = append(weightedPodAffinityTerms, corev1.WeightedPodAffinityTerm{
				Weight:          100,
				PodAffinityTerm: podAffinityTerm,
			})
		}
		podAntiAffinity = &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: weightedPodAffinityTerms,
		}
	}

	// Add pod PodAffinityTerm for dedicated node
	if kbappsv1alpha1.TenancyType(tenancy) == kbappsv1alpha1.DedicatedNode {
		podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(
			podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, corev1.PodAffinityTerm{
				TopologyKey: corev1.LabelHostname,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constant.AppInstanceLabelKey:    clusterName,
						constant.KBAppComponentLabelKey: compName,
					},
				},
			})
	}

	return podAntiAffinity
}

// AddDirToPath add a dir to the PATH environment variable
func AddDirToPath(dir string) error {
	if dir == "" {
		return fmt.Errorf("can't put empty dir into PATH")
	}
	p := strings.TrimSpace(os.Getenv("PATH"))
	dir = strings.TrimSpace(dir)
	if p == "" {
		p = dir
	} else {
		p = dir + ":" + p
	}
	return os.Setenv("PATH", p)
}

func ListResourceByGVR(ctx context.Context, client dynamic.Interface, namespace string, gvrs []schema.GroupVersionResource, selector []metav1.ListOptions, allErrs *[]error) []*unstructured.UnstructuredList {
	unstructuredList := make([]*unstructured.UnstructuredList, 0)
	for _, gvr := range gvrs {
		for _, labelSelector := range selector {
			klog.V(1).Infof("listResourceByGVR: namespace=%s, gvr=%v, selector=%v", namespace, gvr, labelSelector)
			resource, err := client.Resource(gvr).Namespace(namespace).List(ctx, labelSelector)
			if err != nil {
				AppendErrIgnoreNotFound(allErrs, err)
				continue
			}
			unstructuredList = append(unstructuredList, resource)
		}
	}
	return unstructuredList
}

func AppendErrIgnoreNotFound(allErrs *[]error, err error) {
	if err == nil || apierrors.IsNotFound(err) {
		return
	}
	*allErrs = append(*allErrs, err)
}

func WritePogStreamingLog(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod, logOptions corev1.PodLogOptions, writer io.Writer) error {
	request := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &logOptions)
	if data, err := request.DoRaw(ctx); err != nil {
		return err
	} else {
		_, err := writer.Write(data)
		return err
	}
}

// RandRFC1123String generate a random string with length n, which fulfills RFC1123
func RandRFC1123String(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[mrand.Intn(len(letters))]
	}
	return string(b)
}

func TrimVersionPrefix(version string) string {
	version = strings.TrimSpace(version)
	if len(version) > 0 && (version[0] == 'v' || version[0] == 'V') {
		return version[1:]
	}
	return version
}

func GetClusterNameFromArgsOrFlag(cmd *cobra.Command, args []string) string {
	clusterName, _ := cmd.Flags().GetString("cluster")
	if clusterName != "" {
		return clusterName
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func SetHelmOwner(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, releaseName, namespace string, names []string) error {
	patchOP := fmt.Sprintf(`[{"op": "replace", "path": "/metadata/annotations/meta.helm.sh~1release-name", "value": "%s"}`+
		`,{"op": "replace", "path": "/metadata/annotations/meta.helm.sh~1release-namespace", "value": "%s"}]`, releaseName, namespace)
	for _, name := range names {
		if _, err := dynamicClient.Resource(gvr).Namespace("").Patch(context.TODO(), name,
			k8sapitypes.JSONPatchType, []byte(patchOP), metav1.PatchOptions{}); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

// AddAnnotationToComponentOrShard adds a specific annotation to a component.
func AddAnnotationToComponentOrShard(dynamicClient dynamic.Interface, componentName, namespace, annotationKey, annotationValue string) error {
	gvr := types.ComponentGVR()
	resourceClient := dynamicClient.Resource(gvr).Namespace(namespace)
	componentObj, err := resourceClient.Get(context.Background(), componentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get component %s: %v", componentName, err)
	}

	annotations := componentObj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[annotationKey] = annotationValue
	componentObj.SetAnnotations(annotations)

	_, err = resourceClient.Update(context.Background(), componentObj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update component %s with annotation %s: %v", componentName, annotationKey, err)
	}

	return nil
}

func GetComponentsOrShards(cluster *kbappsv1.Cluster) []string {
	var components []string
	for _, component := range cluster.Spec.ComponentSpecs {
		components = append(components, component.Name)
	}
	for _, sharding := range cluster.Spec.Shardings {
		components = append(components, sharding.Name)
	}
	return components
}
