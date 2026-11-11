/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package accounts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/exec"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/cmd/util/podcmd"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"

	clusterutil "github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
)

const (
	defaultPasswordUser          = "postgres"
	defaultPasswordDatabase      = "postgres"
	defaultMongoPasswordUser     = "root"
	defaultMongoPasswordDatabase = "admin"
)

var errMissingPassword = errors.New("please specify password")

type ChangePasswordOptions struct {
	*AccountBaseOptions
	UserName                     string
	Password                     string
	DatabaseName                 string
	SecretName                   string
	AuthSecretName               string
	AccountRootSecretWaitTimeout time.Duration

	targetSecret  *corev1.Secret
	targetSecrets []*corev1.Secret
	authSecret    *corev1.Secret
	oldPassword   string
	oldPasswords  map[string]string
	authUser      string
	authPassword  string

	kubeClient      kubernetes.Interface
	passwordChanger passwordChanger
	readPassword    func() ([]byte, error)
}

func NewChangePasswordOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *ChangePasswordOptions {
	return &ChangePasswordOptions{
		AccountBaseOptions: NewAccountBaseOptions(f, streams),
	}
}

func (o *ChangePasswordOptions) AddFlags(cmd *cobra.Command) {
	o.AccountBaseOptions.AddFlags(cmd)
	cmd.Flags().StringVar(&o.UserName, "name", "", "Specify the database account whose password should be changed. Defaults to postgres for PostgreSQL and root for MongoDB.")
	cmd.Flags().StringVarP(&o.Password, "password", "p", "", "Specify the new password; prompts interactively when omitted.")
	cmd.Flags().StringVar(&o.DatabaseName, "database", "", "Database used for authentication and the password change. Defaults to postgres for PostgreSQL and admin for MongoDB.")
	cmd.Flags().StringVar(&o.SecretName, "secret", "", "Secret to update; when omitted, all related account Secrets are detected and updated.")
	cmd.Flags().StringVar(&o.AuthSecretName, "auth-secret", "", "Secret used for database login; defaults to the target Secret.")
	cmd.Flags().DurationVar(&o.AccountRootSecretWaitTimeout, "account-root-secret-timeout", defaultMongoAccountRootSecretWaitTimeout, "Maximum time to wait for a MongoDB account-root Secret to be deleted and recreated.")
	cmd.Flags().StringVar(&o.ContainerName, "container", "", "Container used for the database command; defaults to the Pod's default container.")
}

func (o *ChangePasswordOptions) Validate() error {
	if o.Password != "" && bytes.IndexByte([]byte(o.Password), 0) >= 0 {
		return errors.New("password cannot contain a NUL byte")
	}
	if bytes.IndexByte([]byte(o.UserName), 0) >= 0 {
		return errors.New("name cannot contain a NUL byte")
	}
	if bytes.IndexByte([]byte(o.DatabaseName), 0) >= 0 {
		return errors.New("database cannot contain a NUL byte")
	}
	if o.AccountRootSecretWaitTimeout < 0 {
		return errors.New("account-root-secret-timeout cannot be negative")
	}
	return nil
}

func (o *ChangePasswordOptions) Complete() error {
	explicitInstance := o.PodName != ""
	if err := o.AccountBaseOptions.Complete(); err != nil {
		return err
	}
	o.setDatabaseDefaults()
	if err := o.validateComponentStatus(); err != nil {
		return err
	}

	ctx := context.Background()
	if !explicitInstance {
		pod, err := findPasswordPod(ctx, o.kube(), o.Namespace, o.ClusterName, o.ComponentName)
		if err != nil {
			return err
		}
		o.Pod = pod
		o.PodName = pod.Name
		o.ExecOptions.Pod = pod
	}

	if err := o.completeSecrets(ctx); err != nil {
		return err
	}
	if o.ContainerName == "" {
		container, err := podcmd.FindOrDefaultContainerByName(o.Pod, "", true, o.ErrOut)
		if err != nil {
			return err
		}
		o.ContainerName = container.Name
	}
	return nil
}

func (o *ChangePasswordOptions) validateComponentStatus() error {
	cluster, err := clusterutil.GetClusterByName(o.Dynamic, o.ClusterName, o.Namespace)
	if err != nil {
		return fmt.Errorf("get cluster %s/%s status: %w", o.Namespace, o.ClusterName, err)
	}
	componentStatus, ok := cluster.Status.Components[o.ComponentName]
	if !ok {
		return fmt.Errorf("component status %s not found in cluster %s", o.ComponentName, o.ClusterName)
	}
	return validatePasswordComponentPhase(o.ComponentName, componentStatus.Phase)
}

func validatePasswordComponentPhase(componentName string, phase appsv1alpha1.ClusterComponentPhase) error {
	if phase != appsv1alpha1.RunningClusterCompPhase {
		return fmt.Errorf("component %s is not running (phase %s), please try again later", componentName, phase)
	}
	return nil
}

func (o *ChangePasswordOptions) completeSecrets(ctx context.Context) error {
	targetSecrets, err := findTargetPasswordSecrets(ctx, o.kube(), o.Namespace, o.ClusterName, o.ComponentName, o.UserName, o.SecretName)
	if err != nil {
		return err
	}
	o.targetSecrets = targetSecrets
	o.targetSecret = targetSecrets[0]

	authSecretName := o.AuthSecretName
	if authSecretName == "" {
		authSecretName = o.targetSecret.Name
	}
	authSecret, err := o.kube().CoreV1().Secrets(o.Namespace).Get(ctx, authSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get auth Secret %s/%s: %w", o.Namespace, authSecretName, err)
	}
	o.authSecret = authSecret

	o.oldPasswords = make(map[string]string, len(targetSecrets))
	for _, targetSecret := range targetSecrets {
		targetUser, err := secretValue(targetSecret, constant.AccountNameForSecret)
		if err != nil {
			return fmt.Errorf("read target Secret %s/%s: %w", o.Namespace, targetSecret.Name, err)
		}
		if !strings.EqualFold(string(targetUser), o.UserName) {
			return fmt.Errorf("Secret %s/%s belongs to user %q, not %q", o.Namespace, targetSecret.Name, targetUser, o.UserName)
		}

		oldPassword, err := secretValue(targetSecret, constant.AccountPasswdForSecret)
		if err != nil {
			return fmt.Errorf("read target Secret %s/%s: %w", o.Namespace, targetSecret.Name, err)
		}
		o.oldPasswords[targetSecret.Name] = string(oldPassword)
		if targetSecret.Name == o.targetSecret.Name {
			o.oldPassword = string(oldPassword)
		}
	}

	authUser, err := secretValue(authSecret, constant.AccountNameForSecret)
	if err != nil {
		return fmt.Errorf("read auth Secret %s/%s: %w", o.Namespace, authSecret.Name, err)
	}
	authPassword, err := secretValue(authSecret, constant.AccountPasswdForSecret)
	if err != nil {
		return fmt.Errorf("read auth Secret %s/%s: %w", o.Namespace, authSecret.Name, err)
	}
	o.authUser = string(authUser)
	o.authPassword = string(authPassword)
	return nil
}

func (o *ChangePasswordOptions) Run() error {
	if o.passwordChanger == nil {
		var err error
		o.passwordChanger, err = newPasswordChanger(o.CharType, passwordChangerConfig{
			Config:    o.Config,
			Client:    o.kube(),
			Executor:  o.Executor,
			Namespace: o.Namespace,
			Database:  o.databaseName(),
			Container: o.ContainerName,
		})
		if err != nil {
			return err
		}
	}

	request := passwordChangeRequest{
		PodName:      o.PodName,
		AuthUser:     o.authUser,
		AuthPassword: o.authPassword,
		TargetUser:   o.UserName,
		NewPassword:  o.Password,
	}
	if err := o.passwordChanger.ChangePassword(request); err != nil {
		o.printGeneralInfo("fail", err.Error())
		return fmt.Errorf("%s rejected the password change; the Secret was not modified: %w", databaseTypeName(o.CharType), err)
	}

	targetSecrets := o.passwordTargetSecrets()
	if len(targetSecrets) == 0 {
		return fmt.Errorf("no target Secret is available; run completion or pass --secret explicitly")
	}
	mutableSecrets := make([]*corev1.Secret, 0, len(targetSecrets))
	immutableSecrets := make([]*corev1.Secret, 0, len(targetSecrets))
	for _, targetSecret := range targetSecrets {
		if secretIsImmutable(targetSecret) {
			immutableSecrets = append(immutableSecrets, targetSecret)
		} else {
			mutableSecrets = append(mutableSecrets, targetSecret)
		}
	}

	updatedSecrets := make([]*corev1.Secret, 0, len(targetSecrets))
	for _, targetSecret := range mutableSecrets {
		if err := patchPasswordSecret(context.Background(), o.kube(), o.Namespace, targetSecret.Name, []byte(o.Password)); err != nil {
			rollbackAuthPassword := o.authPassword
			if strings.EqualFold(o.authUser, o.UserName) {
				rollbackAuthPassword = o.Password
			}
			rollbackErr := o.passwordChanger.ChangePassword(passwordChangeRequest{
				PodName:      o.PodName,
				AuthUser:     o.authUser,
				AuthPassword: rollbackAuthPassword,
				TargetUser:   o.UserName,
				NewPassword:  o.oldPassword,
			})
			restoreErr := o.restorePasswordSecrets(context.Background(), updatedSecrets)
			if rollbackErr != nil {
				return fmt.Errorf("failed to update target Secret %s/%s and failed to roll back the %s password: %w (rollback error: %v, Secret restore error: %v)", o.Namespace, targetSecret.Name, databaseTypeName(o.CharType), err, rollbackErr, restoreErr)
			}
			if restoreErr != nil {
				return fmt.Errorf("failed to update target Secret %s/%s; the %s password was rolled back but related Secrets could not be restored: %w (Secret restore error: %v)", o.Namespace, targetSecret.Name, databaseTypeName(o.CharType), err, restoreErr)
			}
			return fmt.Errorf("failed to update target Secret %s/%s; the %s password was rolled back: %w", o.Namespace, targetSecret.Name, databaseTypeName(o.CharType), err)
		}
		updatedSecrets = append(updatedSecrets, targetSecret)
	}
	if err := o.ensureMongoAccountRootRecreationSource(context.Background(), immutableSecrets); err != nil {
		o.printGeneralInfo("fail", err.Error())
		return fmt.Errorf("password changed, but failed to prepare MongoDB account-root Secret recreation: %w", err)
	}
	deletedImmutableSecrets := make([]*corev1.Secret, 0, len(immutableSecrets))
	stuckAccountRootSecrets := make([]*corev1.Secret, 0, len(immutableSecrets))
	for _, targetSecret := range immutableSecrets {
		if err := deletePasswordSecret(context.Background(), o.kube(), o.Namespace, targetSecret.Name); err != nil {
			o.printGeneralInfo("fail", err.Error())
			return fmt.Errorf("password changed and mutable target Secrets were updated, but failed to delete immutable Secret %s/%s for KubeBlocks to recreate: %w", o.Namespace, targetSecret.Name, err)
		}
		if err := o.waitForMongoAccountRootSecret(targetSecret); err != nil {
			stuckAccountRootSecrets = append(stuckAccountRootSecrets, targetSecret)
			continue
		}
		deletedImmutableSecrets = append(deletedImmutableSecrets, targetSecret)
	}

	o.printGeneralInfo("success", fmt.Sprintf("password for account %q changed successfully", o.UserName))
	if len(deletedImmutableSecrets) > 0 {
		printer.Alert(o.Out, "The new password is stored in mutable Secret(s); immutable Secret(s) were deleted for KubeBlocks to recreate.\n")
	}
	if len(stuckAccountRootSecrets) > 0 {
		for _, targetSecret := range stuckAccountRootSecrets {
			timeout := o.AccountRootSecretWaitTimeout
			if timeout <= 0 {
				timeout = defaultMongoAccountRootSecretWaitTimeout
			}
			printer.Warning(o.Out, "MongoDB password changed successfully, but deletion and recreation of account-root Secret %s/%s was not confirmed within %s. The deletion may be blocked by a finalizer. If the Secret remains stuck, run:\n  %s\n", o.Namespace, targetSecret.Name, timeout, mongoAccountRootSecretFinalizerPatchCommand(o.Namespace, targetSecret.Name))
		}
	}
	if len(immutableSecrets) == 0 {
		printer.Alert(o.Out, "The new password is stored in the target Secret(s).\n")
	}
	return nil
}

func (o *ChangePasswordOptions) passwordTargetSecrets() []*corev1.Secret {
	if len(o.targetSecrets) > 0 {
		return o.targetSecrets
	}
	if o.targetSecret != nil {
		return []*corev1.Secret{o.targetSecret}
	}
	return nil
}

func (o *ChangePasswordOptions) restorePasswordSecrets(ctx context.Context, targetSecrets []*corev1.Secret) error {
	for _, targetSecret := range targetSecrets {
		oldPassword, ok := o.oldPasswords[targetSecret.Name]
		if !ok && o.targetSecret != nil && targetSecret.Name == o.targetSecret.Name {
			oldPassword = o.oldPassword
			ok = oldPassword != ""
		}
		if !ok {
			return fmt.Errorf("old password for Secret %s/%s is unavailable", o.Namespace, targetSecret.Name)
		}
		if err := patchPasswordSecret(ctx, o.kube(), o.Namespace, targetSecret.Name, []byte(oldPassword)); err != nil {
			return err
		}
	}
	return nil
}

func secretIsImmutable(secret *corev1.Secret) bool {
	return secret != nil && secret.Immutable != nil && *secret.Immutable
}

func (o *ChangePasswordOptions) kube() kubernetes.Interface {
	if o.kubeClient != nil {
		return o.kubeClient
	}
	return o.Client
}

func (o *ChangePasswordOptions) setDatabaseDefaults() {
	if o.UserName == "" {
		o.UserName = defaultPasswordUserForCharacterType(o.CharType)
	}
	if o.DatabaseName == "" {
		o.DatabaseName = defaultPasswordDatabaseForCharacterType(o.CharType)
	}
}

func (o *ChangePasswordOptions) databaseName() string {
	if o.DatabaseName != "" {
		return o.DatabaseName
	}
	return defaultPasswordDatabaseForCharacterType(o.CharType)
}

func (o *ChangePasswordOptions) Exec() error {
	if err := o.Validate(); err != nil {
		return err
	}
	if err := o.Complete(); err != nil {
		return err
	}
	if o.Password == "" {
		if err := o.promptPassword(); err != nil {
			return err
		}
	}
	return o.Run()
}

func (o *ChangePasswordOptions) promptPassword() error {
	readPassword := o.readPassword
	if readPassword == nil {
		readPassword = func() ([]byte, error) {
			return terminal.ReadPassword(int(os.Stdin.Fd()))
		}
	}
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	fmt.Fprint(out, "New password: ")
	passwordBytes, err := readPassword()
	fmt.Fprintln(out)
	if err != nil {
		return fmt.Errorf("read new password: %w; specify --password for non-interactive use", err)
	}
	password := string(passwordBytes)
	if err := validatePassword(password); err != nil {
		return fmt.Errorf("read new password: %w", err)
	}

	fmt.Fprint(out, "Confirm password: ")
	confirmationBytes, err := readPassword()
	fmt.Fprintln(out)
	if err != nil {
		return fmt.Errorf("confirm new password: %w; specify --password for non-interactive use", err)
	}
	confirmation := string(confirmationBytes)
	if err := validatePassword(confirmation); err != nil {
		return fmt.Errorf("confirm new password: %w", err)
	}
	if confirmation != password {
		return errors.New("confirm new password: passwords do not match")
	}
	o.Password = password
	return nil
}

func validatePassword(password string) error {
	if password == "" {
		return errMissingPassword
	}
	if bytes.IndexByte([]byte(password), 0) >= 0 {
		return errors.New("password cannot contain a NUL byte")
	}
	return nil
}

type passwordChangeRequest struct {
	PodName      string
	AuthUser     string
	AuthPassword string
	TargetUser   string
	NewPassword  string
}

type passwordChanger interface {
	ChangePassword(request passwordChangeRequest) error
}

type passwordChangerConfig struct {
	Config    *rest.Config
	Client    kubernetes.Interface
	Executor  exec.RemoteExecutor
	Namespace string
	Database  string
	Container string
}

type passwordChangerFactory func(config passwordChangerConfig) passwordChanger

var passwordChangerFactories = map[string]passwordChangerFactory{
	"postgres": func(config passwordChangerConfig) passwordChanger {
		return &postgresPasswordChanger{config: config}
	},
	"postgresql": func(config passwordChangerConfig) passwordChanger {
		return &postgresPasswordChanger{config: config}
	},
	"mongo": func(config passwordChangerConfig) passwordChanger {
		return &mongoPasswordChanger{config: config}
	},
	"mongodb": func(config passwordChangerConfig) passwordChanger {
		return &mongoPasswordChanger{config: config}
	},
}

func newPasswordChanger(characterType string, config passwordChangerConfig) (passwordChanger, error) {
	factory, ok := passwordChangerFactories[strings.ToLower(characterType)]
	if !ok {
		return nil, fmt.Errorf("changing passwords for database type %q is not supported", characterType)
	}
	return factory(config), nil
}

func executePasswordCommand(config passwordChangerConfig, podName string, command []string, stdin io.Reader, stdinEnabled bool, clientName string) error {
	execRequest := config.Client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(config.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: config.Container,
			Command:   command,
			Stdin:     stdinEnabled,
			Stdout:    true,
			Stderr:    true,
		}, k8sscheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	err := config.Executor.Execute(
		"POST",
		execRequest.URL(),
		config.Config,
		stdin,
		&stdout,
		&stderr,
		false,
		nil,
	)
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("execute %s: %w: %s", clientName, err, message)
		}
		return fmt.Errorf("execute %s: %w", clientName, err)
	}
	return nil
}

func findTargetPasswordSecret(ctx context.Context, client kubernetes.Interface, namespace, cluster, component, user, explicitName string) (*corev1.Secret, error) {
	targetSecrets, err := findTargetPasswordSecrets(ctx, client, namespace, cluster, component, user, explicitName)
	if err != nil {
		return nil, err
	}
	return targetSecrets[0], nil
}

func findTargetPasswordSecrets(ctx context.Context, client kubernetes.Interface, namespace, cluster, component, user, explicitName string) ([]*corev1.Secret, error) {
	secrets := client.CoreV1().Secrets(namespace)
	if explicitName != "" {
		secret, err := secrets.Get(ctx, explicitName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get target Secret %s/%s: %w", namespace, explicitName, err)
		}
		return []*corev1.Secret{secret}, nil
	}

	candidates := make([]string, 0, 4)
	addCandidate := func(name string) {
		if name == "" {
			return
		}
		for _, candidate := range candidates {
			if candidate == name {
				return
			}
		}
		candidates = append(candidates, name)
	}

	accountName := strings.ReplaceAll(user, "_", "-")
	if strings.EqualFold(user, defaultPasswordUser) || strings.EqualFold(user, defaultMongoPasswordUser) {
		addCandidate(cluster + "-conn-credential")
	}
	addCandidate(cluster + "-" + component + "-account-" + accountName)

	selector := labels.Set{
		constant.AppInstanceLabelKey:    cluster,
		constant.ClusterAccountLabelKey: user,
	}.AsSelector().String()
	list, err := secrets.List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil && !apierrors.IsForbidden(err) {
		return nil, fmt.Errorf("list account Secrets: %w", err)
	}
	if err == nil {
		sort.Slice(list.Items, func(i, j int) bool {
			return list.Items[i].Name < list.Items[j].Name
		})
		for i := range list.Items {
			addCandidate(list.Items[i].Name)
		}
	}

	targetSecrets := make([]*corev1.Secret, 0, len(candidates))
	for _, candidate := range candidates {
		secret, err := secrets.Get(ctx, candidate, metav1.GetOptions{})
		if err == nil {
			targetSecrets = append(targetSecrets, secret)
			continue
		}
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("get candidate Secret %s/%s: %w", namespace, candidate, err)
		}
	}
	if len(targetSecrets) == 0 {
		return nil, fmt.Errorf("cannot detect the target Secret; pass --secret explicitly")
	}
	return targetSecrets, nil
}

func findPasswordPod(ctx context.Context, client kubernetes.Interface, namespace, cluster, component string) (*corev1.Pod, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Pods: %w", err)
	}
	return selectPasswordPod(list.Items, cluster, component)
}

func selectPasswordPod(pods []corev1.Pod, cluster, component string) (*corev1.Pod, error) {
	preferred := make([]corev1.Pod, 0)
	fallback := make([]corev1.Pod, 0)
	for i := range pods {
		pod := pods[i]
		if pod.Status.Phase != corev1.PodRunning || !passwordPodMatchesComponent(pod, cluster, component) {
			continue
		}
		role := pod.Labels[constant.RoleLabelKey]
		if role == "primary" || role == "leader" {
			preferred = append(preferred, pod)
		} else {
			fallback = append(fallback, pod)
		}
	}

	sort.Slice(preferred, func(i, j int) bool { return preferred[i].Name < preferred[j].Name })
	sort.Slice(fallback, func(i, j int) bool { return fallback[i].Name < fallback[j].Name })
	if len(preferred) == 1 {
		return &preferred[0], nil
	}
	if len(preferred) > 1 {
		return nil, fmt.Errorf("multiple primary/leader Pods found: %s", passwordPodNames(preferred))
	}
	if len(fallback) == 1 {
		return &fallback[0], nil
	}
	if len(fallback) > 1 {
		return nil, fmt.Errorf("cannot identify a unique primary Pod; pass --instance explicitly (candidates: %s)", passwordPodNames(fallback))
	}
	return nil, fmt.Errorf("no Running database Pod found for cluster %q and component %q", cluster, component)
}

func defaultPasswordUserForCharacterType(characterType string) string {
	if isMongoCharacterType(characterType) {
		return defaultMongoPasswordUser
	}
	return defaultPasswordUser
}

func defaultPasswordDatabaseForCharacterType(characterType string) string {
	if isMongoCharacterType(characterType) {
		return defaultMongoPasswordDatabase
	}
	return defaultPasswordDatabase
}

func databaseTypeName(characterType string) string {
	if characterType == "" {
		return "database"
	}
	return characterType
}

func passwordPodMatchesComponent(pod corev1.Pod, cluster, component string) bool {
	if clusterLabel := pod.Labels[constant.AppInstanceLabelKey]; clusterLabel != "" && clusterLabel != cluster {
		return false
	}
	if !strings.HasPrefix(pod.Name, cluster+"-") {
		return false
	}
	if pod.Labels[constant.KBAppComponentLabelKey] == component || pod.Labels[constant.AppComponentLabelKey] == component {
		return true
	}
	return strings.Contains(pod.Name, "-"+component+"-")
}

func passwordPodNames(pods []corev1.Pod) string {
	names := make([]string, 0, len(pods))
	for _, pod := range pods {
		names = append(names, pod.Name)
	}
	return strings.Join(names, ", ")
}

func secretValue(secret *corev1.Secret, key string) ([]byte, error) {
	value, ok := secret.Data[key]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("missing non-empty %q field", key)
	}
	return append([]byte(nil), value...), nil
}

func patchPasswordSecret(ctx context.Context, client kubernetes.Interface, namespace, name string, password []byte) error {
	patch, err := json.Marshal(map[string]map[string]string{
		"data": {
			constant.AccountPasswdForSecret: base64.StdEncoding.EncodeToString(password),
		},
	})
	if err != nil {
		return fmt.Errorf("marshal Secret patch: %w", err)
	}
	if _, err = client.CoreV1().Secrets(namespace).Patch(ctx, name, k8stypes.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("patch Secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

func deletePasswordSecret(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	if err := client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete Secret %s/%s: %w", namespace, name, err)
	}
	return nil
}
