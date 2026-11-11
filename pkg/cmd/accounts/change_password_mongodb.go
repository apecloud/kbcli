package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/apecloud/kubeblocks/pkg/constant"
)

const (
	mongoPasswordShellCommand = `if command -v mongosh >/dev/null 2>&1; then
	exec mongosh --host 127.0.0.1 --port 27017 --quiet
elif command -v mongo >/dev/null 2>&1; then
	exec mongo --host 127.0.0.1 --port 27017 --quiet
else
	echo "neither mongosh nor mongo is installed" >&2
	exit 127
fi
`
	defaultMongoAccountRootSecretWaitTimeout = 30 * time.Second
	mongoAccountRootSecretPollInterval       = time.Second
)

type mongoPasswordChanger struct {
	config passwordChangerConfig
}

func (c *mongoPasswordChanger) ChangePassword(request passwordChangeRequest) error {
	database := c.config.Database
	if database == "" {
		database = defaultMongoPasswordDatabase
	}

	command := []string{"sh", "-c", mongoPasswordShellCommand}
	script := buildMongoPasswordScript(database, request.AuthUser, request.AuthPassword, request.TargetUser, request.NewPassword)
	return executePasswordCommand(c.config, request.PodName, command, strings.NewReader(script), true, "MongoDB client")
}

func (o *ChangePasswordOptions) waitForMongoAccountRootSecret(targetSecret *corev1.Secret) error {
	if !isMongoAccountRootSecret(o.CharType, o.ClusterName, o.ComponentName, o.UserName, targetSecret) {
		return nil
	}
	timeout := o.AccountRootSecretWaitTimeout
	if timeout <= 0 {
		timeout = defaultMongoAccountRootSecretWaitTimeout
	}
	return waitForMongoAccountRootSecretRecreated(
		context.Background(),
		o.kube(),
		o.Namespace,
		targetSecret.Name,
		targetSecret.UID,
		[]byte(o.Password),
		timeout,
		mongoAccountRootSecretPollInterval,
	)
}

func (o *ChangePasswordOptions) ensureMongoAccountRootRecreationSource(ctx context.Context, immutableSecrets []*corev1.Secret) error {
	for _, targetSecret := range immutableSecrets {
		if !isMongoAccountRootSecret(o.CharType, o.ClusterName, o.ComponentName, o.UserName, targetSecret) {
			continue
		}

		secretName := constant.GenerateDefaultConnCredential(o.ClusterName)
		secrets := o.kube().CoreV1().Secrets(o.Namespace)
		sourceSecret, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			sourceSecret, err = secrets.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: o.Namespace,
					Name:      secretName,
					Labels: map[string]string{
						constant.AppInstanceLabelKey:    o.ClusterName,
						constant.ClusterAccountLabelKey: defaultMongoPasswordUser,
					},
				},
				Data: map[string][]byte{
					constant.AccountNameForSecret:   []byte(defaultMongoPasswordUser),
					constant.AccountPasswdForSecret: []byte(o.Password),
				},
			}, metav1.CreateOptions{})
			if err == nil {
				return nil
			}
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("create MongoDB legacy connection Secret %s/%s: %w", o.Namespace, secretName, err)
			}
			sourceSecret, err = secrets.Get(ctx, secretName, metav1.GetOptions{})
		}
		if err != nil {
			return fmt.Errorf("get MongoDB legacy connection Secret %s/%s: %w", o.Namespace, secretName, err)
		}
		if secretIsImmutable(sourceSecret) {
			if bytes.Equal(sourceSecret.Data[constant.AccountPasswdForSecret], []byte(o.Password)) {
				return nil
			}
			return fmt.Errorf("MongoDB legacy connection Secret %s/%s is immutable and does not contain the new password", o.Namespace, secretName)
		}
		if bytes.Equal(sourceSecret.Data[constant.AccountPasswdForSecret], []byte(o.Password)) {
			return nil
		}
		if err := patchPasswordSecret(ctx, o.kube(), o.Namespace, secretName, []byte(o.Password)); err != nil {
			return fmt.Errorf("update MongoDB legacy connection Secret %s/%s: %w", o.Namespace, secretName, err)
		}
		return nil
	}
	return nil
}

func isMongoAccountRootSecret(characterType, cluster, component, user string, secret *corev1.Secret) bool {
	return isMongoCharacterType(characterType) &&
		strings.EqualFold(user, defaultMongoPasswordUser) &&
		secret != nil &&
		secret.Name == constant.GenerateAccountSecretName(cluster, component, defaultMongoPasswordUser)
}

func isMongoCharacterType(characterType string) bool {
	return strings.EqualFold(characterType, "mongo") || strings.EqualFold(characterType, "mongodb")
}

func waitForMongoAccountRootSecretRecreated(ctx context.Context, client kubernetes.Interface, namespace, name string, oldUID k8stypes.UID, expectedPassword []byte, timeout, pollInterval time.Duration) error {
	if oldUID == "" {
		return fmt.Errorf("account-root Secret %s/%s has no UID; cannot verify recreation", namespace, name)
	}
	if timeout <= 0 {
		timeout = defaultMongoAccountRootSecretWaitTimeout
	}
	if pollInterval <= 0 {
		pollInterval = mongoAccountRootSecretPollInterval
	}

	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return false, nil
		case err != nil:
			return false, err
		}
		if secret.DeletionTimestamp != nil || secret.UID == oldUID {
			return false, nil
		}
		return bytes.Equal(secret.Data[constant.AccountPasswdForSecret], expectedPassword), nil
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("account-root Secret %s/%s was not deleted and recreated within %s", namespace, name, timeout)
	}
	return fmt.Errorf("wait for account-root Secret %s/%s to be deleted and recreated: %w", namespace, name, err)
}

func mongoAccountRootSecretFinalizerPatchCommand(namespace, name string) string {
	return fmt.Sprintf("kubectl patch secret %s -n %s --type=merge -p '{\"metadata\":{\"finalizers\":[]}}'", name, namespace)
}

func buildMongoPasswordScript(database, authUser, authPassword, targetUser, newPassword string) string {
	return fmt.Sprintf(
		"var admin = db.getSiblingDB(\"admin\"); var ping = admin.runCommand({ping: 1}); if (!ping || ping.ok !== 1) { printjson(ping); throw new Error(\"MongoDB health check failed: ping\"); } var hello = admin.runCommand({hello: 1}); if (!hello || hello.ok !== 1) { hello = admin.runCommand({isMaster: 1}); } if (!hello || hello.ok !== 1 || (hello.isWritablePrimary !== true && hello.ismaster !== true)) { printjson(hello); throw new Error(\"MongoDB is not a writable primary\"); } var auth = db.getSiblingDB(%s).auth(%s, %s); if (!auth) { throw new Error(\"authentication failed\"); } var result = db.getSiblingDB(%s).runCommand({updateUser: %s, pwd: %s}); if (!result || result.ok !== 1) { printjson(result); throw new Error(\"updateUser failed\"); }",
		quoteMongoJavaScriptString(database),
		quoteMongoJavaScriptString(authUser),
		quoteMongoJavaScriptString(authPassword),
		quoteMongoJavaScriptString(database),
		quoteMongoJavaScriptString(targetUser),
		quoteMongoJavaScriptString(newPassword),
	)
}

func quoteMongoJavaScriptString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
