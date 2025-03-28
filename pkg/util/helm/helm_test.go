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

package helm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/apecloud/kbcli/version"

	"github.com/apecloud/kbcli/pkg/testing"
	"github.com/apecloud/kbcli/pkg/types"
)

var _ = Describe("helm util", func() {
	It("add and remove repo", func() {
		r := repo.Entry{
			Name: "test-repo",
			URL:  "https://test-kubebllcks.com/test-repo",
		}
		Expect(AddRepo(&r)).Should(HaveOccurred())
		Expect(RemoveRepo(&r)).Should(Succeed())
	})

	It("Action Config", func() {
		cfg := NewConfig("test", "config", "context", false)
		actionCfg, err := NewActionConfig(cfg)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(actionCfg).ShouldNot(BeNil())
	})

	Context("Install", func() {
		var o *InstallOpts
		var cfg *Config
		var actionCfg *action.Configuration

		BeforeEach(func() {
			o = &InstallOpts{
				Name:      testing.KubeBlocksChartName,
				Chart:     testing.KubeBlocksChartURL,
				Namespace: "default",
				Version:   version.DefaultKubeBlocksVersion,
			}
			cfg = NewFakeConfig("default")
			actionCfg, _ = NewActionConfig(cfg)
			Expect(actionCfg).ShouldNot(BeNil())
		})

		AfterEach(func() {
			ResetFakeActionConfig()
		})

		It("Install", func() {
			_, err := o.Install(cfg)
			Expect(err).Should(HaveOccurred())
			Expect(o.Uninstall(cfg)).Should(HaveOccurred()) // release not found
		})

		It("should ignore when chart is already deployed", func() {
			err := actionCfg.Releases.Create(&release.Release{
				Name:    o.Name,
				Version: 1,
				Info: &release.Info{
					Status: release.StatusDeployed,
				},
			})
			Expect(err).Should(BeNil())
			_, err = o.tryInstall(actionCfg)
			Expect(err).Should(BeNil())
			Expect(o.tryUninstall(actionCfg)).Should(BeNil()) // release exists
		})

		It("should fail when chart is failed installed", func() {
			err := actionCfg.Releases.Create(&release.Release{
				Name:    o.Name,
				Version: 1,
				Info: &release.Info{
					Status: release.StatusFailed,
				},
			})
			Expect(err).Should(BeNil())
			_, err = o.Install(cfg)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("Upgrade", func() {
		var o *InstallOpts
		var cfg *Config
		var actionCfg *action.Configuration

		BeforeEach(func() {
			o = &InstallOpts{
				Name:      types.KubeBlocksChartName,
				Chart:     "kubeblocks-test-chart",
				Namespace: "default",
				Version:   version.DefaultKubeBlocksVersion,
			}
			cfg = NewFakeConfig("default")
			actionCfg, _ = NewActionConfig(cfg)
			Expect(actionCfg).ShouldNot(BeNil())
		})

		AfterEach(func() {
			ResetFakeActionConfig()
		})

		It("should fail when release is not found", func() {
			Expect(ReleaseNotFound(o.Upgrade(cfg))).Should(BeTrue())
			Expect(o.Uninstall(cfg)).Should(HaveOccurred()) // release not found
		})

		It("should fail when status is not one of deployed, failed and superseded.", func() {
			testCase := []struct {
				status      release.Status
				checkResult bool
			}{
				// deployed, failed and superseded
				{release.StatusDeployed, true},
				{release.StatusSuperseded, true},
				{release.StatusFailed, true},
				// others
				{release.StatusUnknown, false},
				{release.StatusUninstalled, false},
				{release.StatusUninstalling, false},
				{release.StatusPendingInstall, false},
				{release.StatusPendingUpgrade, false},
				{release.StatusPendingRollback, false},
			}

			for i := range testCase {
				err := actionCfg.Releases.Create(&release.Release{
					Name:    o.Name,
					Version: 1,
					Info: &release.Info{
						Status: testCase[i].status,
					},
					Chart: &chart.Chart{},
				})
				Expect(err).Should(BeNil())
				_, err = o.tryUpgrade(actionCfg)
				if testCase[i].checkResult {
					Expect(errors.Is(err, ErrReleaseNotReadyForUpgrade)).Should(BeFalse())
				} else {
					Expect(errors.Is(err, ErrReleaseNotReadyForUpgrade)).Should(BeTrue())
				}
				Expect(o.tryUninstall(actionCfg)).Should(BeNil()) // release exists
			}
		})
	})

	It("get chart versions", func() {
		versions, _ := GetChartVersions(testing.KubeBlocksChartName)
		Expect(versions).Should(BeNil())
	})

	It("get helm release status", func() {
		var o *InstallOpts
		var cfg *Config
		var actionCfg *action.Configuration

		o = &InstallOpts{
			Name:      types.KubeBlocksChartName,
			Chart:     "kubeblocks-test-chart",
			Namespace: "default",
			Version:   version.DefaultKubeBlocksVersion,
		}
		cfg = NewFakeConfig("default")
		actionCfg, _ = NewActionConfig(cfg)
		Expect(actionCfg).ShouldNot(BeNil())

		err := actionCfg.Releases.Create(&release.Release{
			Name:    o.Name,
			Version: 1,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
			Chart: &chart.Chart{},
		})
		Expect(err).Should(BeNil())
		_, _ = GetValues("", cfg)
		res, err := GetHelmRelease(cfg, o.Name)
		Expect(res.Info.Status).Should(Equal(release.StatusFailed))
		Expect(err).Should(BeNil())
		ResetFakeActionConfig()
	})

})
