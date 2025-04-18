/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conversion

import (
	"context"

	"k8s.io/client-go/dynamic"

	"github.com/apecloud/kbcli/pkg/util"
)

type VersionConversionMeta struct {
	dynamic.Interface
	Ctx context.Context

	FromVersion string
	ToVersion   string
}

func (version VersionConversionMeta) NeedConversion() bool {
	return version.FromVersion != version.ToVersion
}

func NewVersionConversion(dynamic dynamic.Interface, fromVersion, toVersion string) *VersionConversionMeta {
	return &VersionConversionMeta{
		Ctx:         context.Background(),
		Interface:   dynamic,
		FromVersion: util.GetMajorMinorVersion(fromVersion),
		ToVersion:   util.GetMajorMinorVersion(toVersion),
	}
}
