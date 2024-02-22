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

package alert

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/util"
)

var (
	deleteReceiverExample = templates.Examples(`
		# delete a receiver named my-receiver, all receivers can be found by command: kbcli alert list-receivers
		kbcli alert delete-receiver my-receiver`)
)

type DeleteReceiverOptions struct {
	baseOptions
	Names []string
}

func NewDeleteReceiverOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *DeleteReceiverOptions {
	return &DeleteReceiverOptions{baseOptions: baseOptions{Factory: f, IOStreams: streams}}
}

func newDeleteReceiverCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewDeleteReceiverOption(f, streams)
	cmd := &cobra.Command{
		Use:     "delete-receiver NAME",
		Short:   "Delete alert receiver.",
		Example: deleteReceiverExample,
		Run: func(cmd *cobra.Command, args []string) {
			o.Names = args
			util.CheckErr(o.Exec())
		},
	}
	return cmd
}

func (o *DeleteReceiverOptions) Exec() error {
	if err := o.complete(); err != nil {
		return err
	}
	if err := o.validate(); err != nil {
		return err
	}
	if err := o.run(); err != nil {
		return err
	}
	return nil
}

func (o *DeleteReceiverOptions) validate() error {
	if len(o.Names) == 0 {
		return fmt.Errorf("receiver name is required")
	}
	return nil
}

func (o *DeleteReceiverOptions) run() error {
	// delete receiver from alert manager config
	if err := o.deleteReceiver(); err != nil {
		return err
	}

	// delete receiver from webhook config
	if err := o.deleteWebhookReceivers(); err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "Receiver %s deleted successfully\n", strings.Join(o.Names, ","))
	return nil
}

func (o *DeleteReceiverOptions) deleteReceiver() error {
	data, err := getConfigData(o.alertConfigMap, o.AlertConfigFileName)
	if err != nil {
		return err
	}

	var newTimeIntervals []interface{}
	var newReceivers []interface{}
	var newRoutes []interface{}

	timeIntervals := getTimeIntervalsFromData(data)
	for i, ti := range timeIntervals {
		var found bool
		name := ti.(map[string]interface{})["name"].(string)
		for _, n := range o.Names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			newTimeIntervals = append(newTimeIntervals, timeIntervals[i])
		}
	}

	// build receiver route map, key is receiver name, value is route
	receiverRouteMap := make(map[string]interface{})
	routes := getRoutesFromData(data)
	for i, r := range routes {
		name := r.(map[string]interface{})["receiver"].(string)
		receiverRouteMap[name] = routes[i]
	}

	receivers := getReceiversFromData(data)
	for i, rec := range receivers {
		var found bool
		name := rec.(map[string]interface{})["name"].(string)
		for _, n := range o.Names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			newReceivers = append(newReceivers, receivers[i])
			r, ok := receiverRouteMap[name]
			if !ok {
				klog.V(1).Infof("receiver %s not found in routes\n", name)
				continue
			}
			newRoutes = append(newRoutes, r)
		}
	}

	// check if receiver exists
	if len(receivers) == len(newReceivers) {
		return fmt.Errorf("receiver %s not found", strings.Join(o.Names, ","))
	}

	data["time_intervals"] = newTimeIntervals
	data["receivers"] = newReceivers
	data["route"].(map[string]interface{})["routes"] = newRoutes
	return updateConfig(o.client, o.alertConfigMap, o.AlertConfigFileName, data)
}

func (o *DeleteReceiverOptions) deleteWebhookReceivers() error {
	data, err := getConfigData(o.webhookConfigMap, webhookAdaptorFileName)
	if err != nil {
		return err
	}
	var newReceivers []interface{}
	receivers := getReceiversFromData(data)
	for i, rec := range receivers {
		var found bool
		name := rec.(map[string]interface{})["name"].(string)
		for _, n := range o.Names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			newReceivers = append(newReceivers, receivers[i])
		}
	}
	data["receivers"] = newReceivers
	return updateConfig(o.client, o.webhookConfigMap, webhookAdaptorFileName, data)
}
