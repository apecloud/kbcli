/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

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

package helm

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

var utf8bom = []byte{0xEF, 0xBB, 0xBF}

func LoadChartFromSource(f fs.FS, name string) (*chart.Chart, error) {
	var files []*loader.BufferedFile
	topdir := name + string(filepath.Separator)
	walk := func(path string, entry fs.DirEntry, err error) error {
		n := strings.TrimPrefix(path, topdir)
		if n == "" {
			// No need to process top level. Avoid bug with helmignore .* matching
			// empty names. See issue 1779.
			return nil
		}

		// Normalize to / since it will also work on Windows
		n = filepath.ToSlash(n)

		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		// Irregular files include devices, sockets, and other uses of files that
		// are not regular files. In Go they have a file mode type bit set.
		// See https://golang.org/pkg/os/#FileMode for examples.
		if !entry.Type().IsRegular() {
			return fmt.Errorf("cannot load irregular file %s as it has file mode type bits set", entry.Name())
		}

		data, err := fs.ReadFile(f, path)
		if err != nil {
			return errors.Wrapf(err, "error reading %s", n)
		}

		data = bytes.TrimPrefix(data, utf8bom)

		files = append(files, &loader.BufferedFile{Name: n, Data: data})
		return nil
	}
	if err := fs.WalkDir(f, name, walk); err != nil {
		return nil, err
	}
	return loader.LoadFiles(files)
}
