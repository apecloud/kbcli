#!/usr/bin/env bash
#
# Copyright (C) 2022-2024 ApeCloud Co., Ltd
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_RESOURCE="clusters.apps.kubeblocks.io"
COMPONENT_RESOURCE="components.apps.kubeblocks.io"
CLUSTER_DEFINITION_RESOURCE="clusterdefinitions.apps.kubeblocks.io"
COMPONENT_DEFINITION_RESOURCE="componentdefinitions.apps.kubeblocks.io"

NAMESPACE=""
SCOPE=""
CONTEXT=""
USERNAME="postgres"
PASSWORD_BYTES=24
PASSWORD_FILE=""
DRY_RUN=false
RUNNING_ONLY=false
CONTINUE_ON_ERROR=false
KUBECTL="kubectl"
KBCLI="kbcli"
OPENSSL="openssl"
JQ="jq"

WORK_DIR=""
RESULT_TMP=""

show_help() {
  cat <<'SHOW_HELP_EOF'
Usage: rotate-postgres-passwords.sh [options]

Rotate the PostgreSQL account password for every PostgreSQL component managed
by KubeBlocks in a Kubernetes cluster. A unique password is generated for each
target and recorded in a local TSV file with mode 0600.

Options:
  -n, --namespace NAME     Only scan this namespace. Mutually exclusive with
                            --scope.
      --scope SCOPE        Namespace selection scope: all or system. all scans
                            every namespace (default); system scans namespaces
                            whose names do not start with ns-. Mutually
                            exclusive with --namespace.
      --context NAME       Kubernetes context used by kubectl and kbcli.
      --username NAME      PostgreSQL account to rotate (default: postgres).
      --password-bytes N   Random bytes used for each hexadecimal password
                            (default: 24; allowed range: 16-128).
      --password-file FILE Output TSV file. Default: a new file under TMPDIR.
      --kubectl PATH       kubectl executable (default: kubectl).
      --kbcli PATH         kbcli executable (default: kbcli).
      --openssl PATH       openssl executable (default: openssl).
      --jq PATH            jq executable (default: jq).
      --dry-run            Print discovered targets without changing passwords.
      --running-only       Only include components whose Cluster is Running.
      --continue-on-error  Continue after a failed rotation.
  -h, --help               Show this help.

The output TSV has this header:

  namespace<TAB>cluster<TAB>component<TAB>username<TAB>password<TAB>result

Passwords are written before kbcli is invoked. If kbcli fails after changing
the database but before returning success, the result remains check-manually;
keep the generated password until the target has been inspected.

Targets are discovered from resolved Component resources and ComponentDefinition
serviceKind values. For older KubeBlocks versions, the script falls back to
Cluster componentSpecs and ClusterDefinition characterType values.
SHOW_HELP_EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "executable not found: $1"
}

cleanup() {
  if [[ -n "$WORK_DIR" ]]; then
    rm -rf -- "$WORK_DIR"
  fi
  if [[ -n "$RESULT_TMP" ]]; then
    rm -f -- "$RESULT_TMP"
  fi
}

while (($# > 0)); do
  case "$1" in
    -n|--namespace)
      [[ $# -ge 2 ]] || die "--namespace requires a value"
      NAMESPACE=$2
      shift 2
      ;;
    --namespace=*)
      NAMESPACE=${1#*=}
      shift
      ;;
    --scope)
      [[ $# -ge 2 ]] || die "--scope requires a value"
      SCOPE=$2
      shift 2
      ;;
    --scope=*)
      SCOPE=${1#*=}
      shift
      ;;
    --context)
      [[ $# -ge 2 ]] || die "--context requires a value"
      CONTEXT=$2
      shift 2
      ;;
    --context=*)
      CONTEXT=${1#*=}
      shift
      ;;
    --username)
      [[ $# -ge 2 ]] || die "--username requires a value"
      USERNAME=$2
      shift 2
      ;;
    --username=*)
      USERNAME=${1#*=}
      shift
      ;;
    --password-bytes)
      [[ $# -ge 2 ]] || die "--password-bytes requires a value"
      PASSWORD_BYTES=$2
      shift 2
      ;;
    --password-bytes=*)
      PASSWORD_BYTES=${1#*=}
      shift
      ;;
    --password-file)
      [[ $# -ge 2 ]] || die "--password-file requires a value"
      PASSWORD_FILE=$2
      shift 2
      ;;
    --password-file=*)
      PASSWORD_FILE=${1#*=}
      shift
      ;;
    --kubectl)
      [[ $# -ge 2 ]] || die "--kubectl requires a value"
      KUBECTL=$2
      shift 2
      ;;
    --kubectl=*)
      KUBECTL=${1#*=}
      shift
      ;;
    --kbcli)
      [[ $# -ge 2 ]] || die "--kbcli requires a value"
      KBCLI=$2
      shift 2
      ;;
    --kbcli=*)
      KBCLI=${1#*=}
      shift
      ;;
    --openssl)
      [[ $# -ge 2 ]] || die "--openssl requires a value"
      OPENSSL=$2
      shift 2
      ;;
    --openssl=*)
      OPENSSL=${1#*=}
      shift
      ;;
    --jq)
      [[ $# -ge 2 ]] || die "--jq requires a value"
      JQ=$2
      shift 2
      ;;
    --jq=*)
      JQ=${1#*=}
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --running-only)
      RUNNING_ONLY=true
      shift
      ;;
    --continue-on-error)
      CONTINUE_ON_ERROR=true
      shift
      ;;
    -h|--help)
      show_help
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      die "unexpected argument: $1"
      ;;
  esac
done

if (($# > 0)); then
  die "unexpected argument: $1"
fi

[[ "$SCOPE" == "" || "$SCOPE" == "all" || "$SCOPE" == "system" ]] ||
  die "scope must be all or system"
[[ -z "$NAMESPACE" || -z "$SCOPE" ]] ||
  die "--namespace and --scope are mutually exclusive"
[[ -n "$SCOPE" ]] || SCOPE=all
[[ -n "$USERNAME" ]] || die "username must not be empty"
[[ "$USERNAME" != *$'\t'* && "$USERNAME" != *$'\n'* && "$USERNAME" != *$'\r'* ]] ||
  die "username must not contain tabs or newlines"
[[ "$PASSWORD_BYTES" =~ ^[0-9]+$ ]] || die "password bytes must be an integer"
((PASSWORD_BYTES >= 16 && PASSWORD_BYTES <= 128)) ||
  die "password bytes must be between 16 and 128"

require_command "$KUBECTL"
require_command "$JQ"
if [[ "$DRY_RUN" == false ]]; then
  require_command "$KBCLI"
  require_command "$OPENSSL"
fi

umask 077
WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/rotate-postgres-passwords.XXXXXX")
trap cleanup EXIT
trap 'exit 130' HUP INT TERM

CONTEXT_ARGS=()
NAMESPACE_ARGS=(-A)
if [[ -n "$CONTEXT" ]]; then
  CONTEXT_ARGS=(--context "$CONTEXT")
fi
if [[ -n "$NAMESPACE" ]]; then
  NAMESPACE_ARGS=(-n "$NAMESPACE")
fi

fetch_resource() {
  local resource=$1
  local output=$2

  "$KUBECTL" "${CONTEXT_ARGS[@]}" get "$resource" "${NAMESPACE_ARGS[@]}" -o json >"$output"
  "$JQ" -e '(.items | type) == "array"' "$output" >/dev/null
}

CLUSTERS_JSON="$WORK_DIR/clusters.json"
COMPONENTS_JSON="$WORK_DIR/components.json"
CLUSTER_DEFINITIONS_JSON="$WORK_DIR/cluster-definitions.json"
COMPONENT_DEFINITIONS_JSON="$WORK_DIR/component-definitions.json"

fetch_resource "$CLUSTER_RESOURCE" "$CLUSTERS_JSON" ||
  die "failed to list $CLUSTER_RESOURCE"

COMPONENTS_OK=true
COMPONENT_DEFINITIONS_OK=true
CLUSTER_DEFINITIONS_OK=true
if ! fetch_resource "$COMPONENT_RESOURCE" "$COMPONENTS_JSON"; then
  COMPONENTS_OK=false
  printf '{"items":[]}\n' >"$COMPONENTS_JSON"
  warn "failed to list $COMPONENT_RESOURCE; trying Cluster-based discovery"
fi
if ! fetch_resource "$COMPONENT_DEFINITION_RESOURCE" "$COMPONENT_DEFINITIONS_JSON"; then
  COMPONENT_DEFINITIONS_OK=false
  printf '{"items":[]}\n' >"$COMPONENT_DEFINITIONS_JSON"
  warn "failed to list $COMPONENT_DEFINITION_RESOURCE"
fi
if ! fetch_resource "$CLUSTER_DEFINITION_RESOURCE" "$CLUSTER_DEFINITIONS_JSON"; then
  CLUSTER_DEFINITIONS_OK=false
  printf '{"items":[]}\n' >"$CLUSTER_DEFINITIONS_JSON"
  warn "failed to list $CLUSTER_DEFINITION_RESOURCE"
fi

if [[ "$COMPONENTS_OK" == false || "$COMPONENT_DEFINITIONS_OK" == false ]] &&
  [[ "$CLUSTER_DEFINITIONS_OK" == false ]]; then
  die "neither ComponentDefinition-based nor legacy ClusterDefinition-based discovery is available"
fi

TARGETS_FILE="$WORK_DIR/targets.tsv"
"$JQ" -rn \
  --slurpfile clusters "$CLUSTERS_JSON" \
  --slurpfile components "$COMPONENTS_JSON" \
  --slurpfile clusterDefinitions "$CLUSTER_DEFINITIONS_JSON" \
  --slurpfile componentDefinitions "$COMPONENT_DEFINITIONS_JSON" \
  --argjson runningOnly "$RUNNING_ONLY" \
  --arg namespaceScope "$SCOPE" \
  '
  def clusterIsRunning($namespace; $clusterName):
    any($clusters[0].items[];
        .metadata.namespace == $namespace and
        .metadata.name == $clusterName and
        .status.phase == "Running");

  def namespaceMatchesScope($namespace):
    $namespaceScope == "all" or
      ($namespaceScope == "system" and (($namespace | startswith("ns-")) | not));

  [
    $components[0].items[]
    | .metadata.namespace as $namespace
    | (.metadata.labels["app.kubernetes.io/instance-name"] // "") as $clusterName
    | (.metadata.labels["apps.kubeblocks.io/component-name"] // "") as $componentName
    | (.metadata.labels["apps.kubeblocks.io/component-def-ref"] // "") as $componentDefinition
    | select($clusterName != "" and $componentName != "" and $componentDefinition != "")
    | select(namespaceMatchesScope($namespace))
    | select(any($componentDefinitions[0].items[];
                 .metadata.name == $componentDefinition and
                 ((.spec.serviceKind // "" | ascii_downcase) == "postgresql")))
    | select(($runningOnly | not) or clusterIsRunning($namespace; $clusterName))
    | {namespace: $namespace, cluster: $clusterName, component: $componentName}
  ]
  +
  [
    $clusters[0].items[]
    | .metadata.namespace as $namespace
    | .metadata.name as $clusterName
    | .spec.clusterDefinitionRef as $clusterDefinition
    | .spec.componentSpecs[]?
    | (.componentDef // "") as $componentDefinition
    | (.name // "") as $componentName
    | select($componentDefinition != "" and $componentName != "")
    | select(namespaceMatchesScope($namespace))
    | select(any($componentDefinitions[0].items[];
                 .metadata.name == $componentDefinition and
                 ((.spec.serviceKind // "" | ascii_downcase) == "postgresql")))
    | select(($runningOnly | not) or clusterIsRunning($namespace; $clusterName))
    | {namespace: $namespace, cluster: $clusterName, component: $componentName}
  ]
  +
  [
    $clusters[0].items[]
    | .metadata.namespace as $namespace
    | .metadata.name as $clusterName
    | .spec.clusterDefinitionRef as $clusterDefinition
    | .spec.componentSpecs[]?
    | (.componentDefRef // "") as $componentDefinition
    | (.name // "") as $componentName
    | select($clusterDefinition != null and $componentDefinition != "" and $componentName != "")
    | select(namespaceMatchesScope($namespace))
    | select(any($clusterDefinitions[0].items[];
                 .metadata.name == $clusterDefinition and
                 any(.spec.componentDefs[]?;
                     (.name // "") == $componentDefinition and
                     ((.characterType // "" | ascii_downcase) == "postgresql"))))
    | select(($runningOnly | not) or clusterIsRunning($namespace; $clusterName))
    | {namespace: $namespace, cluster: $clusterName, component: $componentName}
  ]
  | unique_by([.namespace, .cluster, .component])
  | sort_by([.namespace, .cluster, .component])[]
  | [.namespace, .cluster, .component]
  | @tsv
' >"$TARGETS_FILE"

TARGET_COUNT=$(wc -l <"$TARGETS_FILE" | tr -d '[:space:]')
if [[ "$DRY_RUN" == true ]]; then
  printf 'namespace\tcluster\tcomponent\n'
  cat "$TARGETS_FILE"
  printf 'Discovered %s PostgreSQL component(s); no passwords were changed.\n' "$TARGET_COUNT"
  exit 0
fi

if ((TARGET_COUNT == 0)); then
  printf 'No PostgreSQL components were found; no passwords were changed.\n'
  exit 0
fi

if [[ -z "$PASSWORD_FILE" ]]; then
  PASSWORD_FILE=$(mktemp "${TMPDIR:-/tmp}/rotate-postgres-passwords.XXXXXX")
else
  if [[ -e "$PASSWORD_FILE" || -L "$PASSWORD_FILE" ]]; then
    die "password file already exists: $PASSWORD_FILE"
  fi
  : >"$PASSWORD_FILE"
fi
chmod 0600 "$PASSWORD_FILE"
RESULT_TMP="${PASSWORD_FILE}.tmp.$$"
printf 'namespace\tcluster\tcomponent\tusername\tpassword\tresult\n' >"$PASSWORD_FILE"

printf 'Rotating %d PostgreSQL password(s). Results: %s\n' "$TARGET_COUNT" "$PASSWORD_FILE"
FAILURES=0
LINE_NUMBER=2
while IFS=$'\t' read -r NAMESPACE_NAME CLUSTER_NAME COMPONENT_NAME; do
  PASSWORD=$("$OPENSSL" rand -hex "$PASSWORD_BYTES")
  [[ "$PASSWORD" =~ ^[0-9a-fA-F]+$ ]] || die "openssl returned an invalid password"

  printf '%s\t%s\t%s\t%s\t%s\tpending\n' \
    "$NAMESPACE_NAME" "$CLUSTER_NAME" "$COMPONENT_NAME" "$USERNAME" "$PASSWORD" \
    >>"$PASSWORD_FILE"

  printf 'Rotating %s/%s/%s...\n' "$NAMESPACE_NAME" "$CLUSTER_NAME" "$COMPONENT_NAME"
  KBCLI_ARGS=("${CONTEXT_ARGS[@]}" --namespace "$NAMESPACE_NAME")
  if "$KBCLI" "${KBCLI_ARGS[@]}" cluster change-password "$CLUSTER_NAME" \
    --component "$COMPONENT_NAME" \
    --name "$USERNAME" \
    --password "$PASSWORD"; then
    awk -F '\t' -v OFS='\t' -v lineNumber="$LINE_NUMBER" -v result=rotated '
      NR == lineNumber {$6 = result}
      {print}
    ' "$PASSWORD_FILE" >"$RESULT_TMP"
    mv -f "$RESULT_TMP" "$PASSWORD_FILE"
  else
    awk -F '\t' -v OFS='\t' -v lineNumber="$LINE_NUMBER" -v result=check-manually '
      NR == lineNumber {$6 = result}
      {print}
    ' "$PASSWORD_FILE" >"$RESULT_TMP"
    mv -f "$RESULT_TMP" "$PASSWORD_FILE"
    FAILURES=$((FAILURES + 1))
    printf 'Rotation failed for %s/%s/%s; inspect it before retrying. The generated password is recorded as check-manually.\n' \
      "$NAMESPACE_NAME" "$CLUSTER_NAME" "$COMPONENT_NAME" >&2
    if [[ "$CONTINUE_ON_ERROR" == false ]]; then
      exit 1
    fi
  fi
  LINE_NUMBER=$((LINE_NUMBER + 1))
done <"$TARGETS_FILE"

if ((FAILURES > 0)); then
  printf 'Completed with %d failure(s). Results: %s\n' "$FAILURES" "$PASSWORD_FILE" >&2
  exit 1
fi

printf 'Completed %d rotation(s). Results: %s\n' "$TARGET_COUNT" "$PASSWORD_FILE"
