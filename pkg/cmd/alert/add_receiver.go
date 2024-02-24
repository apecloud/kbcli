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

package alert

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/timeinterval"

	"github.com/apecloud/kbcli/pkg/util"
)

var (
	// alertConfigmapName is the name of alertmanager configmap
	alertConfigmapName = getConfigMapName(alertManagerAddonName)

	// webhookAdaptorConfigmapName is the name of webhook adaptor
	webhookAdaptorConfigmapName = getConfigMapName(webhookAdaptorAddonName)
)

var (
	addReceiverExample = templates.Examples(`
		# add webhook receiver without token, for example feishu
		kbcli alert add-receiver --webhook='url=https://open.feishu.cn/open-apis/bot/v2/hook/foo'

		# add webhook receiver with token, for example feishu
		kbcli alert add-receiver --webhook='url=https://open.feishu.cn/open-apis/bot/v2/hook/foo,token=XXX'

		# add email receiver
        kbcli alert add-receiver --email='user1@kubeblocks.io,user2@kubeblocks.io'

		# add email receiver, and only receive alert from cluster mycluster
		kbcli alert add-receiver --email='user1@kubeblocks.io,user2@kubeblocks.io' --cluster=mycluster

		# add email receiver, and only receive alert from cluster mycluster and alert severity is warning
		kbcli alert add-receiver --email='user1@kubeblocks.io,user2@kubeblocks.io' --cluster=mycluster --severity=warning

		# add email receiver, and only receive alert from mysql clusters
		kbcli alert add-receiver --email='user1@kubeblocks.io,user2@kubeblocks.io' --type=mysql

		# add email receiver, and only receive alert triggered by specified rule
		kbcli alert add-receiver --email='user1@kubeblocks.io,user2@kubeblocks.io' --rule=MysqlDown

		# add slack receiver
  		kbcli alert add-receiver --slack api_url=https://hooks.slackConfig.com/services/foo,channel=monitor,username=kubeblocks-alert-bot`)
)

type baseOptions struct {
	genericiooptions.IOStreams
	alertConfigMap      *corev1.ConfigMap
	webhookConfigMap    *corev1.ConfigMap
	client              kubernetes.Interface
	Factory             cmdutil.Factory
	AlertConfigMapKey   apitypes.NamespacedName
	AlertConfigFileName string
	NoAdapter           bool
}

type Times struct {
	StartTime string
	EndTime   string
}

type TimeInterval struct {
	Times    Times
	Weekdays []string
}

type AddReceiverOptions struct {
	baseOptions

	Emails           []string
	Webhooks         []string
	Slacks           []string
	Clusters         []string
	Severities       []string
	Types            []string
	Rules            []string
	Name             string
	InputName        []string
	SendResolved     bool
	RepeatInterval   string
	MuteTimeInterval *timeinterval.TimeInterval

	receiver                *receiver
	route                   *route
	timeInterval            *config.TimeInterval
	webhookAdaptorReceivers []webhookAdaptorReceiver
}

func NewAddReceiverOption(f cmdutil.Factory, streams genericiooptions.IOStreams) *AddReceiverOptions {
	o := AddReceiverOptions{baseOptions: baseOptions{Factory: f, IOStreams: streams}}
	return &o
}

func newAddReceiverCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewAddReceiverOption(f, streams)
	cmd := &cobra.Command{
		Use:     "add-receiver",
		Short:   "Add alert receiver, such as email, slack, webhook and so on.",
		Example: addReceiverExample,
		Run: func(cmd *cobra.Command, args []string) {
			o.InputName = args
			util.CheckErr(o.Exec())
		},
	}

	cmd.Flags().StringArrayVar(&o.Emails, "email", []string{}, "Add email address, such as user@kubeblocks.io, more than one emailConfig can be specified separated by comma")
	cmd.Flags().StringArrayVar(&o.Webhooks, "webhook", []string{}, "Add webhook receiver, such as url=https://open.feishu.cn/open-apis/bot/v2/hook/foo,token=xxxxx")
	cmd.Flags().StringArrayVar(&o.Slacks, "slack", []string{}, "Add slack receiver, such as api_url=https://hooks.slackConfig.com/services/foo,channel=monitor,username=kubeblocks-alert-bot")
	cmd.Flags().StringArrayVar(&o.Clusters, "cluster", []string{}, "Cluster name, such as mycluster, more than one cluster can be specified, such as mycluster1,mycluster2")
	cmd.Flags().StringArrayVar(&o.Severities, "severity", []string{}, "Alert severity level, critical, warning or info, more than one severity level can be specified, such as critical,warning")
	cmd.Flags().StringArrayVar(&o.Types, "type", []string{}, "Engine type, such as mysql, more than one types can be specified, such as mysql,postgresql,redis")
	cmd.Flags().StringArrayVar(&o.Rules, "rule", []string{}, "Rule name, such as MysqlDown, more than one rule names can be specified, such as MysqlDown,MysqlRestarted")
	cmd.Flags().StringVar(&o.RepeatInterval, "repeat-interval", "", "Repeat interval of current receiver")

	// register completions
	util.CheckErr(cmd.RegisterFlagCompletionFunc("severity",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return severities(), cobra.ShellCompDirectiveNoFileComp
		}))

	return cmd
}

func (o *AddReceiverOptions) Exec() error {
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

func (o *baseOptions) complete() error {
	var err error
	ctx := context.Background()

	if o.Factory == nil {
		return errors.Errorf("no factory")
	}
	o.client, err = o.Factory.KubernetesClientSet()
	if err != nil {
		return err
	}

	namespace, err := util.GetKubeBlocksNamespace(o.client)
	if err != nil {
		return err
	}

	if len(o.AlertConfigFileName) == 0 {
		o.AlertConfigFileName = alertConfigFileName
	}
	if len(o.AlertConfigMapKey.Name) == 0 {
		o.AlertConfigMapKey.Name = alertConfigmapName
	}
	if len(o.AlertConfigMapKey.Namespace) == 0 {
		o.AlertConfigMapKey.Namespace = namespace
	}
	// get alertmanager configmap
	o.alertConfigMap, err = o.client.CoreV1().ConfigMaps(o.AlertConfigMapKey.Namespace).Get(ctx, o.AlertConfigMapKey.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// get webhook adaptor configmap
	o.webhookConfigMap, err = o.client.CoreV1().ConfigMaps(namespace).Get(ctx, webhookAdaptorConfigmapName, metav1.GetOptions{})
	return err
}

func (o *AddReceiverOptions) validate() error {
	if len(o.Emails) == 0 && len(o.Webhooks) == 0 && len(o.Slacks) == 0 {
		return fmt.Errorf("must specify at least one receiver, such as --email, --webhook or --slack")
	}

	// if name is not specified, generate a random one
	if len(o.InputName) == 0 {
		o.Name = generateReceiverName()
	} else {
		o.Name = o.InputName[0]
	}

	if err := o.checkEmails(); err != nil {
		return err
	}

	if err := o.checkSeverities(); err != nil {
		return err
	}
	return nil
}

// checkSeverities checks if severity is valid
func (o *AddReceiverOptions) checkSeverities() error {
	if len(o.Severities) == 0 {
		return nil
	}
	checkSeverity := func(severity string) error {
		ss := strings.Split(severity, ",")
		for _, s := range ss {
			if !slices.Contains(severities(), strings.ToLower(strings.TrimSpace(s))) {
				return fmt.Errorf("invalid severity: %s, must be one of %v", s, severities())
			}
		}
		return nil
	}

	for _, severity := range o.Severities {
		if err := checkSeverity(severity); err != nil {
			return err
		}
	}
	return nil
}

// checkEmails checks if email SMTP is configured, if not, do not allow to add email receiver
func (o *AddReceiverOptions) checkEmails() error {
	if len(o.Emails) == 0 {
		return nil
	}

	errMsg := "SMTP %sis not configured, if you want to add email receiver, please use `kbcli alert config-smtpserver` configure it first"
	data, err := getConfigData(o.alertConfigMap, o.AlertConfigFileName)
	if err != nil {
		return err
	}

	if data["global"] == nil {
		return fmt.Errorf(errMsg, "")
	}

	// check smtp config in global
	checkKeys := []string{"smtp_from", "smtp_smarthost", "smtp_auth_username", "smtp_auth_password"}
	checkSMTP := func(key string) error {
		val := data["global"].(map[string]interface{})[key]
		if val == nil || fmt.Sprintf("%v", val) == "" {
			return fmt.Errorf(errMsg, key+" ")
		}
		return nil
	}

	for _, key := range checkKeys {
		if err = checkSMTP(key); err != nil {
			return err
		}
	}
	return nil
}

func (o *AddReceiverOptions) run() error {
	// build time interval
	o.buildTimeInterval()

	// build receiver
	if err := o.buildReceiver(); err != nil {
		return err
	}

	// build route
	o.buildRoute()

	// add alertmanager receiver and route
	if err := o.addReceiver(); err != nil {
		return err
	}

	// add webhook receiver
	if err := o.addWebhookReceivers(); err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "Receiver %s added successfully.\n", o.receiver.Name)
	return nil
}

// buildReceiver builds receiver from receiver options
func (o *AddReceiverOptions) buildReceiver() error {
	webhookConfigs, err := o.buildWebhook()
	if err != nil {
		return err
	}

	slackConfigs, err := buildSlackConfigs(o.Slacks)
	if err != nil {
		return err
	}

	o.receiver = &receiver{
		Name:           o.Name,
		EmailConfigs:   buildEmailConfigs(o.Emails),
		WebhookConfigs: webhookConfigs,
		SlackConfigs:   slackConfigs,
	}
	return nil
}

func (o *AddReceiverOptions) buildRoute() {
	r := &route{
		Receiver:       o.Name,
		Continue:       true,
		RepeatInterval: o.RepeatInterval,
	}

	var clusterArray []string
	var severityArray []string
	var typesArray []string
	var rulesArray []string

	if o.MuteTimeInterval != nil {
		r.MuteTimeIntervals = []string{o.Name}
	}

	splitStr := func(strArray []string, target *[]string) {
		for _, s := range strArray {
			ss := strings.Split(s, ",")
			*target = append(*target, ss...)
		}
	}

	// parse clusters and severities
	splitStr(o.Clusters, &clusterArray)
	splitStr(o.Severities, &severityArray)
	splitStr(o.Types, &typesArray)
	splitStr(o.Rules, &rulesArray)

	// build matchers
	buildMatchers := func(t string, values []string) string {
		if len(values) == 0 {
			return ""
		}
		deValues := removeDuplicateStr(values)
		switch t {
		case routeMatcherClusterType:
			return routeMatcherClusterKey + routeMatcherOperator + strings.Join(deValues, "|")
		case routeMatcherSeverityType:
			return routeMatcherSeverityKey + routeMatcherOperator + strings.Join(deValues, "|")
		case routeMatcherTypeType:
			return routeMatcherTypeKey + routeMatcherOperator + strings.Join(deValues, "|")
		case routeMatcherRuleType:
			return routeMatcherRuleKey + routeMatcherOperator + strings.Join(deValues, "|")
		default:
			return ""
		}
	}

	r.Matchers = append(r.Matchers, buildMatchers(routeMatcherClusterType, clusterArray),
		buildMatchers(routeMatcherSeverityType, severityArray),
		buildMatchers(routeMatcherTypeType, typesArray),
		buildMatchers(routeMatcherRuleType, rulesArray))
	o.route = r
}

func (o *AddReceiverOptions) buildTimeInterval() {
	if o.MuteTimeInterval == nil {
		return
	}
	o.timeInterval = &config.TimeInterval{
		Name:          o.Name,
		TimeIntervals: []timeinterval.TimeInterval{*o.MuteTimeInterval},
	}
}

// addReceiver adds receiver to alertmanager config
func (o *AddReceiverOptions) addReceiver() error {
	data, err := getConfigData(o.alertConfigMap, o.AlertConfigFileName)
	if err != nil {
		return err
	}

	// add time interval
	if o.timeInterval != nil {
		timeIntervals := getTimeIntervalsFromData(data)
		if timeIntervalExists(timeIntervals, o.Name) {
			return fmt.Errorf("timeInterval %s already exists", o.timeInterval.Name)
		}
		timeIntervals = append(timeIntervals, o.timeInterval)
		data["time_intervals"] = timeIntervals
	}

	// add receiver
	receivers := getReceiversFromData(data)
	if receiverExists(receivers, o.Name) {
		return fmt.Errorf("receiver %s already exists", o.receiver.Name)
	}
	receivers = append(receivers, o.receiver)

	// add route
	routes := getRoutesFromData(data)
	routes = append(routes, o.route)

	data["receivers"] = receivers
	data["route"].(map[string]interface{})["routes"] = routes

	// update alertmanager configmap
	return updateConfig(o.client, o.alertConfigMap, o.AlertConfigFileName, data)
}

func (o *AddReceiverOptions) addWebhookReceivers() error {
	data, err := getConfigData(o.webhookConfigMap, webhookAdaptorFileName)
	if err != nil {
		return err
	}

	receivers := getReceiversFromData(data)
	for _, r := range o.webhookAdaptorReceivers {
		receivers = append(receivers, r)
	}
	data["receivers"] = receivers

	// update webhook configmap
	return updateConfig(o.client, o.webhookConfigMap, webhookAdaptorFileName, data)
}

// buildWebhook builds webhookConfig and webhookAdaptorReceiver from webhook options
func (o *AddReceiverOptions) buildWebhook() ([]*webhookConfig, error) {
	var ws []*webhookConfig
	var waReceivers []webhookAdaptorReceiver
	for _, hook := range o.Webhooks {
		m := strToMap(hook)
		if len(m) == 0 {
			return nil, fmt.Errorf("invalid webhook: %s, webhook should be in the format of url=my-url,token=my-token", hook)
		}
		w := webhookConfig{
			MaxAlerts:    10,
			SendResolved: o.SendResolved,
		}
		waReceiver := webhookAdaptorReceiver{Name: o.Name}
		for k, v := range m {
			// check webhookConfig keys
			switch webhookKey(k) {
			case webhookURL:
				if valid, err := urlIsValid(v); !valid {
					return nil, fmt.Errorf("invalid webhook url: %s, %v", v, err)
				}
				if o.NoAdapter {
					w.URL = v
				} else {
					w.URL = getWebhookAdaptorURL(o.Name, o.webhookConfigMap.Namespace)
				}
				webhookType := getWebhookType(v)
				waReceiver.Type = string(webhookType)
				waReceiver.Params.URL = v
			case webhookToken:
				waReceiver.Params.Secret = v
			default:
				return nil, fmt.Errorf("invalid webhook key: %s, webhook key should be one of url and token", k)
			}
		}
		ws = append(ws, &w)
		waReceivers = append(waReceivers, waReceiver)
	}
	o.webhookAdaptorReceivers = waReceivers
	return ws, nil
}

func timeIntervalExists(timeIntervals []interface{}, name string) bool {
	for _, r := range timeIntervals {
		if r == nil {
			continue
		}
		n := r.(map[string]interface{})["name"]
		if n != nil && n.(string) == name {
			return true
		}
	}
	return false
}

func receiverExists(receivers []interface{}, name string) bool {
	for _, r := range receivers {
		n := r.(map[string]interface{})["name"]
		if n != nil && n.(string) == name {
			return true
		}
	}
	return false
}

// buildSlackConfigs builds slackConfig from slack options
func buildSlackConfigs(slacks []string) ([]*slackConfig, error) {
	var ss []*slackConfig
	for _, slackStr := range slacks {
		m := strToMap(slackStr)
		if len(m) == 0 {
			return nil, fmt.Errorf("invalid slack: %s, slack config should be in the format of api_url=my-api-url,channel=my-channel,username=my-username", slackStr)
		}
		s := slackConfig{TitleLink: ""}
		for k, v := range m {
			// check slackConfig keys
			switch slackKey(k) {
			case slackAPIURL:
				if valid, err := urlIsValid(v); !valid {
					return nil, fmt.Errorf("invalid slack api_url: %s, %v", v, err)
				}
				s.APIURL = v
			case slackChannel:
				s.Channel = "#" + v
			case slackUsername:
				s.Username = v
			default:
				return nil, fmt.Errorf("invalid slack config key: %s", k)
			}
		}
		ss = append(ss, &s)
	}
	return ss, nil
}

// buildEmailConfigs builds emailConfig from email options
func buildEmailConfigs(emails []string) []*emailConfig {
	var es []*emailConfig
	for _, email := range emails {
		strs := strings.Split(email, ",")
		for _, str := range strs {
			es = append(es, &emailConfig{To: str})
		}
	}
	return es
}

func updateConfig(client kubernetes.Interface, cm *corev1.ConfigMap, key string, data map[string]interface{}) error {
	newValue, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	_, err = client.CoreV1().ConfigMaps(cm.Namespace).Patch(context.TODO(), cm.Name, apitypes.JSONPatchType,
		[]byte(fmt.Sprintf("[{\"op\": \"replace\", \"path\": \"/data/%s\", \"value\": %s }]",
			key, strconv.Quote(string(newValue)))), metav1.PatchOptions{})
	return err
}
