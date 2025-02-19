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

package backuprepo

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"

	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	trueVal = "true"

	associatedBackupRepoKey = "dataprotection.kubeblocks.io/backup-repo-name"
)

func createPatchData(oldObj, newObj runtime.Object) ([]byte, error) {
	oldData, err := json.Marshal(oldObj)
	if err != nil {
		return nil, err
	}
	newData, err := json.Marshal(newObj)
	if err != nil {
		return nil, err
	}
	return jsonpatch.CreateMergePatch(oldData, newData)
}

func tryConvertLegacyStorageProvider(dynamic dynamic.Interface, name string) (*dpv1alpha1.StorageProvider, error) {
	provider := &dpv1alpha1.StorageProvider{}
	err := util.GetK8SClientObject(dynamic, provider, types.LegacyStorageProviderGVR(), "", name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("storage provider \"%s\" is not found", name)
		}
		return nil, err
	}

	var parametersSchema *dpv1alpha1.ParametersSchema
	if provider.Spec.ParametersSchema != nil {
		parametersSchema = &dpv1alpha1.ParametersSchema{
			OpenAPIV3Schema:  provider.Spec.ParametersSchema.OpenAPIV3Schema,
			CredentialFields: provider.Spec.ParametersSchema.CredentialFields,
		}
	}

	newProvider := &dpv1alpha1.StorageProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:        provider.Name,
			Labels:      provider.Labels,
			Annotations: provider.Annotations,
		},
		Spec: dpv1alpha1.StorageProviderSpec{
			CSIDriverName:                 provider.Spec.CSIDriverName,
			CSIDriverSecretTemplate:       provider.Spec.CSIDriverSecretTemplate,
			StorageClassTemplate:          provider.Spec.StorageClassTemplate,
			PersistentVolumeClaimTemplate: provider.Spec.PersistentVolumeClaimTemplate,
			DatasafedConfigTemplate:       provider.Spec.DatasafedConfigTemplate,
			ParametersSchema:              parametersSchema,
		},
	}
	return newProvider, nil
}

func getStorageProvider(dynamic dynamic.Interface, name string) (*dpv1alpha1.StorageProvider, error) {
	provider := &dpv1alpha1.StorageProvider{}
	err := util.GetK8SClientObject(dynamic, provider, types.StorageProviderGVR(), "", name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return tryConvertLegacyStorageProvider(dynamic, name)
		}
		return nil, err
	}
	return provider, nil
}
