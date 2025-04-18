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

package action

import (
	"fmt"
	"reflect"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/apecloud/kbcli/pkg/util"
)

type OutputOperation func(bool) string

type PatchOptions struct {
	Factory cmdutil.Factory

	// resource names
	Names           []string
	GVR             schema.GroupVersionResource
	OutputOperation OutputOperation

	// following fields are similar to kubectl patch
	PrintFlags  *genericclioptions.PrintFlags
	ToPrinter   func(string) (printers.ResourcePrinter, error)
	Patch       string
	Subresource string

	namespace                    string
	enforceNamespace             bool
	dryRunStrategy               cmdutil.DryRunStrategy
	args                         []string
	builder                      *resource.Builder
	unstructuredClientForMapping func(mapping *meta.RESTMapping) (resource.RESTClient, error)
	fieldManager                 string

	EditBeforeUpdate bool

	genericiooptions.IOStreams
}

func NewPatchOptions(f cmdutil.Factory, streams genericiooptions.IOStreams, gvr schema.GroupVersionResource) *PatchOptions {
	return &PatchOptions{
		Factory:         f,
		GVR:             gvr,
		PrintFlags:      genericclioptions.NewPrintFlags("").WithTypeSetter(scheme.Scheme),
		IOStreams:       streams,
		OutputOperation: patchOperation,
	}
}

func (o *PatchOptions) AddFlags(cmd *cobra.Command) {
	o.PrintFlags.AddFlags(cmd)
	cmdutil.AddDryRunFlag(cmd)
	cmd.Flags().BoolVar(&o.EditBeforeUpdate, "edit", o.EditBeforeUpdate, "Edit the API resource")
}

func (o *PatchOptions) CmdComplete(cmd *cobra.Command) error {
	var err error
	o.dryRunStrategy, err = cmdutil.GetDryRunStrategy(cmd)
	if err != nil {
		return err
	}
	return nil
}

func (o *PatchOptions) complete() error {
	var err error
	if len(o.Names) == 0 {
		return fmt.Errorf("missing %s name", o.GVR.Resource)
	}

	cmdutil.PrintFlagsWithDryRunStrategy(o.PrintFlags, o.dryRunStrategy)
	o.ToPrinter = func(operation string) (printers.ResourcePrinter, error) {
		o.PrintFlags.NamePrintFlags.Operation = operation
		return o.PrintFlags.ToPrinter()
	}

	o.namespace, o.enforceNamespace, err = o.Factory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	o.args = append([]string{util.GVRToString(o.GVR)}, o.Names...)
	o.builder = o.Factory.NewBuilder()
	o.unstructuredClientForMapping = o.Factory.UnstructuredClientForMapping
	return nil
}

func (o *PatchOptions) Run() error {
	if err := o.complete(); err != nil {
		return err
	}

	if len(o.Patch) == 0 {
		return fmt.Errorf("the contents of the patch is empty")
	}

	// for CRD, we always use Merge patch type
	patchType := types.MergePatchType
	patchBytes, err := yaml.ToJSON([]byte(o.Patch))
	if err != nil {
		return fmt.Errorf("unable to parse %q: %v", o.Patch, err)
	}

	r := o.builder.
		Unstructured().
		ContinueOnError().
		NamespaceParam(o.namespace).DefaultNamespace().
		Subresource(o.Subresource).
		ResourceTypeOrNameArgs(false, o.args...).
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}
	count := 0
	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		count++
		name, namespace := info.Name, info.Namespace
		if o.dryRunStrategy != cmdutil.DryRunClient {
			mapping := info.ResourceMapping()
			client, err := o.unstructuredClientForMapping(mapping)
			if err != nil {
				return err
			}

			helper := resource.
				NewHelper(client, mapping).
				DryRun(o.dryRunStrategy == cmdutil.DryRunServer).
				WithFieldManager(o.fieldManager).
				WithSubresource(o.Subresource).
				WithFieldValidation(metav1.FieldValidationStrict)
			patchedObj, err := helper.Patch(namespace, name, patchType, patchBytes, nil)
			if err != nil {
				if apierrors.IsUnsupportedMediaType(err) {
					return errors.Wrap(err, fmt.Sprintf("%s is not supported by %s", patchType, mapping.GroupVersionKind))
				}
				return fmt.Errorf("unable to update %s %s/%s: %v", info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, err)
			}

			if o.EditBeforeUpdate {
				customEdit := NewCustomEditOptions(o.Factory, o.IOStreams, "patched")
				if err = customEdit.Run(patchedObj); err != nil {
					return fmt.Errorf("unable to edit %s %s/%s: %v", info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, err)
				}
				patchedObj = &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": patchedObj.(*unstructured.Unstructured).Object["metadata"],
						"spec":     patchedObj.(*unstructured.Unstructured).Object["spec"],
					},
				}
				patchBytes, err = patchedObj.(*unstructured.Unstructured).MarshalJSON()
				if err != nil {
					return fmt.Errorf("unable to marshal %s %s/%s: %v", info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, err)
				}
				patchedObj, err = helper.Patch(namespace, name, patchType, patchBytes, nil)
				if err != nil {
					if apierrors.IsUnsupportedMediaType(err) {
						return errors.Wrap(err, fmt.Sprintf("%s is not supported by %s", patchType, mapping.GroupVersionKind))
					}
					return fmt.Errorf("unable to update %s %s/%s: %v", info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, err)
				}
			}

			didPatch := !reflect.DeepEqual(info.Object, patchedObj)
			printer, err := o.ToPrinter(o.OutputOperation(didPatch))
			if err != nil {
				return err
			}
			return printer.PrintObj(patchedObj, o.Out)
		}

		originalObjJS, err := runtime.Encode(unstructured.UnstructuredJSONScheme, info.Object)
		if err != nil {
			return err
		}

		originalPatchedObjJS, err := getPatchedJSON(patchType, originalObjJS, patchBytes, info.Object.GetObjectKind().GroupVersionKind(), scheme.Scheme)
		if err != nil {
			return err
		}

		targetObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, originalPatchedObjJS)
		if err != nil {
			return err
		}

		didPatch := !reflect.DeepEqual(info.Object, targetObj)
		printer, err := o.ToPrinter(o.OutputOperation(didPatch))
		if err != nil {
			return err
		}
		return printer.PrintObj(targetObj, o.Out)
	})
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no objects passed to patch")
	}
	return nil
}

func getPatchedJSON(patchType types.PatchType, originalJS, patchJS []byte, gvk schema.GroupVersionKind, oc runtime.ObjectCreater) ([]byte, error) {
	switch patchType {
	case types.JSONPatchType:
		patchObj, err := jsonpatch.DecodePatch(patchJS)
		if err != nil {
			return nil, err
		}
		bytes, err := patchObj.Apply(originalJS)
		// TODO: This is pretty hacky, we need a better structured error from the json-patch
		if err != nil && strings.Contains(err.Error(), "doc is missing key") {
			msg := err.Error()
			ix := strings.Index(msg, "key:")
			key := msg[ix+5:]
			return bytes, fmt.Errorf("object to be patched is missing field (%s)", key)
		}
		return bytes, err

	case types.MergePatchType:
		return jsonpatch.MergePatch(originalJS, patchJS)

	case types.StrategicMergePatchType:
		// get a typed object for this GVK if we need to apply a strategic merge patch
		obj, err := oc.New(gvk)
		if err != nil {
			return nil, fmt.Errorf("strategic merge patch is not supported for %s locally, try --type merge", gvk.String())
		}
		return strategicpatch.StrategicMergePatch(originalJS, patchJS, obj)

	default:
		// only here as a safety net - go-restful filters content-type
		return nil, fmt.Errorf("unknown Content-Type header for patch: %v", patchType)
	}
}

func patchOperation(didPatch bool) string {
	if didPatch {
		return "patched"
	}
	return "patched (no change)"
}
