# JGFOperator

## CRD with ClientSet


### Initialize Project

```bash
MODULE=fluxframework.io/jgfoperator
go mod init $MODULE
kubebuilder init --domain fluxframework.io
kubebuilder edit --multigroup=true
```

### Generate Resources and Manifests

```bash
kubebuilder create api --group flux --version v1 --kind PodInfo

Create Resource[y/n]y

Create Controller[y/n]y

```


**Edit type**

```
make manifests
```

### Docker Build

```bash
make docker-build
```

### Local Deploy

```bash
make docker-push-local

make deploy-local
```


## Pod Controller

```bash
kubebuilder create api --group core --version v1 --kind Pod

Create Resource[y/n]n

Create Controller[y/n]y
```

```bash
make manifests
```

Modify `pod_controller.go`

- It seems that `make docker-build` will automatically run the following cmd
```bash
go get k8s.io/api/core/v1@v0.22.1
```

## Code-Generator

- [source](https://www.fatalerrors.org/a/writing-crd-by-mixing-kubeuilder-and-code-generator.html)


    MODULE and go.mod bring into correspondence with
    API_PKG=apis, consistent with the API directory
    OUTPUT_PKG=generated/flux, the group specified when generating the Resource is the same
    GROUP_VERSION=flux:v1 Corresponds to the group version specified when the Resource is generated

### Prepare script

`hack/tools.go`
```go
// +build tools
package tools

import _ "k8s.io/code-generator"
```

`update-codegen.sh`
```bash
#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# corresponding to go mod init <module>
MODULE=fluxframework.io/jgfoperator
# api package
APIS_PKG=apis
# generated output package
OUTPUT_PKG=generated/flux
# group-version such as foo:v1alpha1
GROUP_VERSION=flux:v1

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash "${CODEGEN_PKG}"/generate-groups.sh "client,lister,informer" \
  ${MODULE}/${OUTPUT_PKG} ${MODULE}/${APIS_PKG} \
  ${GROUP_VERSION} \
  --go-header-file "${SCRIPT_ROOT}"/hack/boilerplate.go.txt \
  --output-base "${SCRIPT_ROOT}"
#  --output-base "${SCRIPT_ROOT}/../../.." \
```

`verify-codegen.sh`
```bash
#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

OUTPUT_PKG=generated/flux
MODULE=fluxframework.io/jgfoperator

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

DIFFROOT="${SCRIPT_ROOT}/${OUTPUT_PKG}"
TMP_DIFFROOT="${SCRIPT_ROOT}/_tmp/${OUTPUT_PKG}"
_tmp="${SCRIPT_ROOT}/_tmp"

cleanup() {
  rm -rf "${_tmp}"
}
trap "cleanup" EXIT SIGINT

cleanup

mkdir -p "${TMP_DIFFROOT}"
cp -a "${DIFFROOT}"/* "${TMP_DIFFROOT}"

"${SCRIPT_ROOT}/hack/update-codegen.sh"
echo "copying generated ${SCRIPT_ROOT}/${MODULE}/${OUTPUT_PKG} to ${DIFFROOT}"
cp -r "${SCRIPT_ROOT}/${MODULE}/${OUTPUT_PKG}"/* "${DIFFROOT}"

echo "diffing ${DIFFROOT} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT}" "${TMP_DIFFROOT}" || ret=$?
cp -a "${TMP_DIFFROOT}"/* "${DIFFROOT}"
if [[ $ret -eq 0 ]]
then
  echo "${DIFFROOT} up to date."
else
  echo "${DIFFROOT} is out of date. Please run hack/update-codegen.sh"
  exit 1
fi
```
