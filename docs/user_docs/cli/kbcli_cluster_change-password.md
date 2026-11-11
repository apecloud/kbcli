---
title: kbcli cluster change-password
---

Change a database account password

When automatically discovered account Secrets are immutable, kbcli deletes them after updating mutable credential Secrets so KubeBlocks can recreate them with the new password. For MongoDB, if only the immutable `<cluster>-<component>-account-root` Secret exists, kbcli creates or updates the legacy `<cluster>-conn-credential` Secret as KubeBlocks' recreation source. It then waits up to 30 seconds for the account-root Secret to be deleted and recreated with the new password. If deletion remains pending, kbcli still reports the password change as successful and prints a `kubectl patch secret ...` command for manually removing the Secret finalizer.

The cluster and selected component must be in the `Running` phase. For PostgreSQL, kbcli verifies that the server is not in recovery mode before changing the password. For MongoDB, kbcli checks connectivity and verifies that the selected server is a writable primary.

```
kbcli cluster change-password [flags]
```

### Examples

```
  # interactively change the default PostgreSQL account password and update its Secret
  kbcli cluster change-password CLUSTERNAME
  # change the default MongoDB root password and update its Secret
  kbcli cluster change-password CLUSTERNAME --password PASSWD
  # change an application role password with explicit Secrets
  kbcli cluster change-password CLUSTERNAME --component COMPNAME --name USERNAME --password PASSWD --secret TARGET_SECRET --auth-secret ADMIN_SECRET
  # change a password on a specified instance
  kbcli cluster change-password --instance INSTANCE --name USERNAME --password PASSWD
```

### Options

```
      --auth-secret string   Secret used for database login; defaults to the target Secret.
      --account-root-secret-timeout duration
                              Maximum time to wait for a MongoDB account-root Secret to be deleted and recreated. (default 30s)
      --component string     Specify the name of component to be connected. If not specified, pick the first one.
      --container string     Container used for the database command; defaults to the Pod's default container.
      --database string      Database used for authentication and the password change. Defaults to postgres for PostgreSQL and admin for MongoDB.
  -h, --help                 help for change-password
  -i, --instance string      Specify the name of instance to be connected.
      --name string          Specify the database account whose password should be changed. Defaults to postgres for PostgreSQL and root for MongoDB.
  -p, --password string      Specify the new password; prompts interactively when omitted.
      --secret string        Secret to update; when omitted, all related account Secrets are detected and updated.
```

### Options inherited from parent commands

```
      --as string                      Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --as-group stringArray           Group to impersonate for the operation, this flag can be repeated to specify multiple groups.
      --as-uid string                  UID to impersonate for the operation.
      --cache-dir string               Default cache directory (default "$HOME/.kube/cache")
      --certificate-authority string   Path to a cert file for the certificate authority
      --client-certificate string      Path to a client certificate file for TLS
      --client-key string              Path to a client key file for TLS
      --cluster string                 The name of the kubeconfig cluster to use
      --context string                 The name of the kubeconfig context to use
      --disable-compression            If true, opt-out of response compression for all requests to the server
      --insecure-skip-tls-verify       If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string              Path to the kubeconfig file to use for CLI requests.
      --match-server-version           Require server version to match client version
  -n, --namespace string               If present, the namespace scope for this CLI request
      --request-timeout string         The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                  The address and port of the Kubernetes API server
      --tls-server-name string         Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used
      --token string                   Bearer token for authentication to the API server
      --user string                    The name of the kubeconfig user to use
```

### SEE ALSO

* [kbcli cluster](kbcli_cluster.md)	 - Cluster command.

#### Go Back to [CLI Overview](cli.md) Homepage.
