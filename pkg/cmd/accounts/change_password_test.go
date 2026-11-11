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
	"io"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
)

func TestBuildPostgresPasswordSQL(t *testing.T) {
	got := buildPostgresPasswordSQL(`role"name`, `p'ass\word`)
	want := "SET standard_conforming_strings = on;\nDO $$ BEGIN IF pg_is_in_recovery() THEN RAISE EXCEPTION 'PostgreSQL is in recovery mode and is not writable'; END IF; END $$;\nALTER ROLE \"role\"\"name\" PASSWORD 'p''ass\\word';\n"
	if got != want {
		t.Fatalf("buildPostgresPasswordSQL() = %q, want %q", got, want)
	}
}

func TestPromptPassword(t *testing.T) {
	streams, _, _, _ := genericiooptions.NewTestIOStreams()
	var output bytes.Buffer
	streams.Out = &output
	o := NewChangePasswordOptions(nil, streams)
	passwords := [][]byte{[]byte("new-password"), []byte("new-password")}
	o.readPassword = func() ([]byte, error) {
		password := passwords[0]
		passwords = passwords[1:]
		return password, nil
	}

	if err := o.promptPassword(); err != nil {
		t.Fatalf("promptPassword() error = %v", err)
	}
	if o.Password != "new-password" {
		t.Fatalf("prompted password = %q, want %q", o.Password, "new-password")
	}
	if strings.Contains(output.String(), "new-password") {
		t.Fatalf("prompt output contains the plaintext password: %q", output.String())
	}
}

func TestBuildMongoPasswordScriptEscapesValues(t *testing.T) {
	got := buildMongoPasswordScript("admin", `admin"user`, `a'p\ass`, `root"user`, `p'ass\word`)
	want := `var admin = db.getSiblingDB("admin"); var ping = admin.runCommand({ping: 1}); if (!ping || ping.ok !== 1) { printjson(ping); throw new Error("MongoDB health check failed: ping"); } var hello = admin.runCommand({hello: 1}); if (!hello || hello.ok !== 1) { hello = admin.runCommand({isMaster: 1}); } if (!hello || hello.ok !== 1 || (hello.isWritablePrimary !== true && hello.ismaster !== true)) { printjson(hello); throw new Error("MongoDB is not a writable primary"); } var auth = db.getSiblingDB("admin").auth("admin\"user", "a'p\\ass"); if (!auth) { throw new Error("authentication failed"); } var result = db.getSiblingDB("admin").runCommand({updateUser: "root\"user", pwd: "p'ass\\word"}); if (!result || result.ok !== 1) { printjson(result); throw new Error("updateUser failed"); }`
	if got != want {
		t.Fatalf("buildMongoPasswordScript() = %q, want %q", got, want)
	}
}

func TestValidatePasswordComponentPhase(t *testing.T) {
	if err := validatePasswordComponentPhase("mongodb", appsv1alpha1.RunningClusterCompPhase); err != nil {
		t.Fatalf("validatePasswordComponentPhase() error = %v", err)
	}

	err := validatePasswordComponentPhase("mongodb", appsv1alpha1.UpdatingClusterCompPhase)
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("validatePasswordComponentPhase() error = %v, want not-running error", err)
	}
}

func TestSelectPasswordPodPrefersPrimary(t *testing.T) {
	pods := []corev1.Pod{
		passwordTestPod("pg-postgresql-1", "replica", "postgresql"),
		passwordTestPod("pg-postgresql-0", "primary", "postgresql"),
	}

	got, err := selectPasswordPod(pods, "pg", "postgresql")
	if err != nil {
		t.Fatalf("selectPasswordPod() error = %v", err)
	}
	if got.Name != "pg-postgresql-0" {
		t.Fatalf("selectPasswordPod() = %q, want %q", got.Name, "pg-postgresql-0")
	}
}

func TestSelectPasswordPodRejectsAmbiguousFallback(t *testing.T) {
	pods := []corev1.Pod{
		passwordTestPod("pg-postgresql-0", "", "postgresql"),
		passwordTestPod("pg-postgresql-1", "", "postgresql"),
	}

	_, err := selectPasswordPod(pods, "pg", "postgresql")
	if err == nil || !strings.Contains(err.Error(), "pass --instance explicitly") {
		t.Fatalf("selectPasswordPod() error = %v, want an ambiguity error", err)
	}
}

func TestFindTargetPasswordSecretUsesDefaultCandidates(t *testing.T) {
	secret := passwordTestSecret("pg-conn-credential", "postgres", "old-password")
	client := fake.NewSimpleClientset(secret)

	got, err := findTargetPasswordSecret(context.Background(), client, "default", "pg", "postgresql", "postgres", "")
	if err != nil {
		t.Fatalf("findTargetPasswordSecret() error = %v", err)
	}
	if got.Name != secret.Name {
		t.Fatalf("findTargetPasswordSecret() = %q, want %q", got.Name, secret.Name)
	}
}

func TestFindTargetPasswordSecretUsesMongoConnectionSecret(t *testing.T) {
	secret := passwordTestSecret("mongo-conn-credential", "root", "old-password")
	client := fake.NewSimpleClientset(secret)

	got, err := findTargetPasswordSecret(context.Background(), client, "default", "mongo", "mongodb", "root", "")
	if err != nil {
		t.Fatalf("findTargetPasswordSecret() error = %v", err)
	}
	if got.Name != secret.Name {
		t.Fatalf("findTargetPasswordSecret() = %q, want %q", got.Name, secret.Name)
	}
}

func TestFindTargetPasswordSecretsUsesMongoConnectionAndAccountSecrets(t *testing.T) {
	connectionSecret := passwordTestSecret("mongo-conn-credential", "root", "old-password")
	accountSecret := passwordTestSecret("mongo-mongodb-account-root", "root", "old-password")
	client := fake.NewSimpleClientset(connectionSecret, accountSecret)

	got, err := findTargetPasswordSecrets(context.Background(), client, "default", "mongo", "mongodb", "root", "")
	if err != nil {
		t.Fatalf("findTargetPasswordSecrets() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != connectionSecret.Name || got[1].Name != accountSecret.Name {
		t.Fatalf("findTargetPasswordSecrets() = %v, want [%q %q]", passwordSecretNames(got), connectionSecret.Name, accountSecret.Name)
	}
}

func TestFindTargetPasswordSecretUsesAccountSecret(t *testing.T) {
	secret := passwordTestSecret("pg-postgresql-account-app-user", "app_user", "old-password")
	secret.Labels = map[string]string{
		constant.AppInstanceLabelKey:    "pg",
		constant.ClusterAccountLabelKey: "app_user",
	}
	client := fake.NewSimpleClientset(secret)

	got, err := findTargetPasswordSecret(context.Background(), client, "default", "pg", "postgresql", "app_user", "")
	if err != nil {
		t.Fatalf("findTargetPasswordSecret() error = %v", err)
	}
	if got.Name != secret.Name {
		t.Fatalf("findTargetPasswordSecret() = %q, want %q", got.Name, secret.Name)
	}
}

func TestPatchPasswordSecret(t *testing.T) {
	secret := passwordTestSecret("target", "appuser", "old-password")
	client := fake.NewSimpleClientset(secret)

	if err := patchPasswordSecret(context.Background(), client, "default", secret.Name, []byte("new-password")); err != nil {
		t.Fatalf("patchPasswordSecret() error = %v", err)
	}

	got, err := client.CoreV1().Secrets("default").Get(context.Background(), secret.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get patched Secret: %v", err)
	}
	if string(got.Data[constant.AccountPasswdForSecret]) != "new-password" {
		t.Fatalf("patched password = %q, want %q", got.Data[constant.AccountPasswdForSecret], "new-password")
	}
}

func TestChangePasswordRunUpdatesSecret(t *testing.T) {
	secret := passwordTestSecret("target", "appuser", "old-password")
	client := fake.NewSimpleClientset(secret)
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, secret, changer)

	if err := o.Run(); err != nil {
		t.Fatalf("ChangePasswordOptions.Run() error = %v", err)
	}
	if len(changer.requests) != 1 {
		t.Fatalf("password change requests = %d, want 1", len(changer.requests))
	}
	if changer.requests[0].NewPassword != "new-password" {
		t.Fatalf("new password = %q, want %q", changer.requests[0].NewPassword, "new-password")
	}

	got, err := client.CoreV1().Secrets("default").Get(context.Background(), secret.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated Secret: %v", err)
	}
	if string(got.Data[constant.AccountPasswdForSecret]) != "new-password" {
		t.Fatalf("updated password = %q, want %q", got.Data[constant.AccountPasswdForSecret], "new-password")
	}
}

func TestChangePasswordRunUpdatesAllTargetSecrets(t *testing.T) {
	connectionSecret := passwordTestSecret("mongo-conn-credential", "appuser", "old-password")
	accountSecret := passwordTestSecret("mongo-mongodb-account-appuser", "appuser", "old-password")
	client := fake.NewSimpleClientset(connectionSecret, accountSecret)
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, connectionSecret, changer)
	o.targetSecrets = []*corev1.Secret{connectionSecret, accountSecret}
	o.oldPasswords = map[string]string{
		connectionSecret.Name: "old-password",
		accountSecret.Name:    "old-password",
	}

	if err := o.Run(); err != nil {
		t.Fatalf("ChangePasswordOptions.Run() error = %v", err)
	}
	for _, name := range []string{connectionSecret.Name, accountSecret.Name} {
		got, err := client.CoreV1().Secrets("default").Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get updated Secret %s: %v", name, err)
		}
		if string(got.Data[constant.AccountPasswdForSecret]) != "new-password" {
			t.Fatalf("updated password in Secret %s = %q, want %q", name, got.Data[constant.AccountPasswdForSecret], "new-password")
		}
	}
}

func TestChangePasswordRunDeletesImmutableTargetSecret(t *testing.T) {
	connectionSecret := passwordTestSecret("mongo-conn-credential", "appuser", "old-password")
	accountSecret := passwordTestSecret("mongo-mongodb-account-appuser", "appuser", "old-password")
	immutable := true
	accountSecret.Immutable = &immutable
	client := fake.NewSimpleClientset(connectionSecret, accountSecret)
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, connectionSecret, changer)
	o.targetSecrets = []*corev1.Secret{connectionSecret, accountSecret}
	o.oldPasswords = map[string]string{
		connectionSecret.Name: "old-password",
		accountSecret.Name:    "old-password",
	}

	if err := o.Run(); err != nil {
		t.Fatalf("ChangePasswordOptions.Run() error = %v", err)
	}
	updated, err := client.CoreV1().Secrets("default").Get(context.Background(), connectionSecret.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated connection Secret: %v", err)
	}
	if string(updated.Data[constant.AccountPasswdForSecret]) != "new-password" {
		t.Fatalf("updated connection password = %q, want %q", updated.Data[constant.AccountPasswdForSecret], "new-password")
	}
	if _, err := client.CoreV1().Secrets("default").Get(context.Background(), accountSecret.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("immutable account Secret lookup error = %v, want NotFound", err)
	}
}

func TestChangePasswordRunWarnsWhenMongoAccountRootSecretDeletionSticks(t *testing.T) {
	connectionSecret := passwordTestSecret("mongo-conn-credential", "root", "old-password")
	accountSecret := passwordTestSecret("mongo-mongodb-account-root", "root", "old-password")
	accountSecret.Immutable = boolptr(true)
	accountSecret.UID = "old-account-root"
	client := fake.NewSimpleClientset(connectionSecret, accountSecret)
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, connectionSecret, changer)
	o.CharType = "mongodb"
	o.ClusterName = "mongo"
	o.ComponentName = "mongodb"
	o.UserName = "root"
	o.targetSecrets = []*corev1.Secret{connectionSecret, accountSecret}
	o.oldPasswords = map[string]string{
		connectionSecret.Name: "old-password",
		accountSecret.Name:    "old-password",
	}
	o.AccountRootSecretWaitTimeout = 5 * time.Millisecond
	var output bytes.Buffer
	o.Out = &output

	if err := o.Run(); err != nil {
		t.Fatalf("ChangePasswordOptions.Run() error = %v", err)
	}
	if !strings.Contains(output.String(), "password changed successfully") {
		t.Fatalf("output = %q, want success message", output.String())
	}
	if !strings.Contains(output.String(), "kubectl patch secret mongo-mongodb-account-root -n default") {
		t.Fatalf("output = %q, want account-root finalizer patch command", output.String())
	}
}

func TestChangePasswordRunSupportsOnlyImmutableMongoAccountRootSecret(t *testing.T) {
	accountSecret := passwordTestSecret("mongo-mongodb-account-root", "root", "old-password")
	accountSecret.Immutable = boolptr(true)
	accountSecret.UID = "old-account-root"
	client := fake.NewSimpleClientset(accountSecret)
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, accountSecret, changer)
	o.CharType = "mongodb"
	o.ClusterName = "mongo"
	o.ComponentName = "mongodb"
	o.UserName = "root"
	o.targetSecrets = []*corev1.Secret{accountSecret}
	o.oldPasswords = map[string]string{accountSecret.Name: "old-password"}
	o.AccountRootSecretWaitTimeout = 5 * time.Millisecond

	if err := o.Run(); err != nil {
		t.Fatalf("ChangePasswordOptions.Run() error = %v", err)
	}
	source, err := client.CoreV1().Secrets("default").Get(context.Background(), "mongo-conn-credential", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get MongoDB legacy connection Secret: %v", err)
	}
	if string(source.Data[constant.AccountNameForSecret]) != "root" {
		t.Fatalf("legacy connection username = %q, want %q", source.Data[constant.AccountNameForSecret], "root")
	}
	if string(source.Data[constant.AccountPasswdForSecret]) != "new-password" {
		t.Fatalf("legacy connection password = %q, want %q", source.Data[constant.AccountPasswdForSecret], "new-password")
	}
}

func TestWaitForMongoAccountRootSecretRecreated(t *testing.T) {
	oldSecret := passwordTestSecret("mongo-mongodb-account-root", "root", "old-password")
	oldSecret.UID = "old-account-root"
	newSecret := oldSecret.DeepCopy()
	newSecret.UID = "new-account-root"
	newSecret.Data[constant.AccountPasswdForSecret] = []byte("new-password")
	client := fake.NewSimpleClientset(oldSecret)
	getCount := 0
	client.PrependReactor("get", "secrets", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		getCount++
		if getCount == 1 {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, oldSecret.Name)
		}
		return true, newSecret, nil
	})

	if err := waitForMongoAccountRootSecretRecreated(context.Background(), client, "default", oldSecret.Name, oldSecret.UID, []byte("new-password"), time.Second, time.Millisecond); err != nil {
		t.Fatalf("waitForMongoAccountRootSecretRecreated() error = %v", err)
	}
}

func TestWaitForMongoAccountRootSecretRecreatedTimesOut(t *testing.T) {
	secret := passwordTestSecret("mongo-mongodb-account-root", "root", "old-password")
	secret.UID = "old-account-root"
	now := metav1.Now()
	secret.DeletionTimestamp = &now
	client := fake.NewSimpleClientset(secret)

	err := waitForMongoAccountRootSecretRecreated(context.Background(), client, "default", secret.Name, secret.UID, []byte("new-password"), 5*time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "was not deleted and recreated") {
		t.Fatalf("waitForMongoAccountRootSecretRecreated() error = %v, want timeout error", err)
	}
}

func boolptr(value bool) *bool {
	return &value
}

func TestChangePasswordRunRollsBackWhenSecretUpdateFails(t *testing.T) {
	client := fake.NewSimpleClientset()
	secret := passwordTestSecret("target", "appuser", "old-password")
	changer := &recordingPasswordChanger{}
	o := newTestChangePasswordOptions(client, secret, changer)

	err := o.Run()
	if err == nil || !strings.Contains(err.Error(), "password was rolled back") {
		t.Fatalf("ChangePasswordOptions.Run() error = %v, want rollback error", err)
	}
	if len(changer.requests) != 2 {
		t.Fatalf("password change requests = %d, want 2", len(changer.requests))
	}
	if changer.requests[1].AuthPassword != "admin-password" {
		t.Fatalf("rollback auth password = %q, want %q", changer.requests[1].AuthPassword, "admin-password")
	}
	if changer.requests[1].NewPassword != "old-password" {
		t.Fatalf("rollback new password = %q, want %q", changer.requests[1].NewPassword, "old-password")
	}
}

func TestNewPasswordChangerRejectsUnsupportedDatabase(t *testing.T) {
	if _, err := newPasswordChanger("mysql", passwordChangerConfig{}); err == nil {
		t.Fatal("newPasswordChanger() should reject unsupported databases")
	}
}

func TestNewPasswordChangerSupportsMongoDB(t *testing.T) {
	for _, characterType := range []string{"mongo", "mongodb"} {
		changer, err := newPasswordChanger(characterType, passwordChangerConfig{})
		if err != nil {
			t.Fatalf("newPasswordChanger(%q) error = %v", characterType, err)
		}
		if _, ok := changer.(*mongoPasswordChanger); !ok {
			t.Fatalf("newPasswordChanger(%q) type = %T, want *mongoPasswordChanger", characterType, changer)
		}
	}
}

func TestDefaultPasswordValuesForCharacterType(t *testing.T) {
	if got := defaultPasswordUserForCharacterType("mongodb"); got != defaultMongoPasswordUser {
		t.Fatalf("MongoDB default user = %q, want %q", got, defaultMongoPasswordUser)
	}
	if got := defaultPasswordDatabaseForCharacterType("mongodb"); got != defaultMongoPasswordDatabase {
		t.Fatalf("MongoDB default database = %q, want %q", got, defaultMongoPasswordDatabase)
	}
	if got := defaultPasswordUserForCharacterType("postgresql"); got != defaultPasswordUser {
		t.Fatalf("PostgreSQL default user = %q, want %q", got, defaultPasswordUser)
	}
	if got := defaultPasswordDatabaseForCharacterType("postgresql"); got != defaultPasswordDatabase {
		t.Fatalf("PostgreSQL default database = %q, want %q", got, defaultPasswordDatabase)
	}
}

type recordingPasswordChanger struct {
	requests []passwordChangeRequest
}

func (c *recordingPasswordChanger) ChangePassword(request passwordChangeRequest) error {
	c.requests = append(c.requests, request)
	return nil
}

func newTestChangePasswordOptions(client *fake.Clientset, secret *corev1.Secret, changer passwordChanger) *ChangePasswordOptions {
	streams, _, _, _ := genericiooptions.NewTestIOStreams()
	o := NewChangePasswordOptions(nil, streams)
	o.kubeClient = client
	o.targetSecret = secret
	o.Password = "new-password"
	o.UserName = "appuser"
	o.Namespace = "default"
	o.PodName = "pg-postgresql-0"
	o.authUser = "admin"
	o.authPassword = "admin-password"
	o.oldPassword = "old-password"
	o.passwordChanger = changer
	o.Out = io.Discard
	o.ErrOut = io.Discard
	return o
}

func passwordTestPod(name, role, component string) corev1.Pod {
	labels := map[string]string{
		constant.AppInstanceLabelKey:    "pg",
		constant.KBAppComponentLabelKey: component,
	}
	if role != "" {
		labels[constant.RoleLabelKey] = role
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func passwordTestSecret(name, user, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data: map[string][]byte{
			constant.AccountNameForSecret:   []byte(user),
			constant.AccountPasswdForSecret: []byte(password),
		},
	}
}

func passwordSecretNames(secrets []*corev1.Secret) []string {
	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		names = append(names, secret.Name)
	}
	return names
}
