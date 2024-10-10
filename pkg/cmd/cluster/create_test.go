/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

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

package cluster

import (
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

var _ = Describe("create", func() {
	Context("multipleSourceComponent test", func() {
		defer GinkgoRecover()
		streams := genericiooptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stdout,
		}
		It("target file stored in website", func() {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte("OK"))
				Expect(err).ShouldNot(HaveOccurred())
			}))
			defer ts.Close()
			fileURL := ts.URL + "/docs/file"
			bytes, err := MultipleSourceComponents(fileURL, streams.In)
			Expect(bytes).Should(Equal([]byte("OK")))
			Expect(err).ShouldNot(HaveOccurred())
		})
		It("target file doesn't exist", func() {
			fileName := "no-existing-file"
			bytes, err := MultipleSourceComponents(fileName, streams.In)
			Expect(bytes).Should(BeNil())
			Expect(err).Should(HaveOccurred())
		})
	})

})
