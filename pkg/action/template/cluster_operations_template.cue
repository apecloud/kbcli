//Copyright (C) 2022-2024 ApeCloud Co., Ltd
//
//This file is part of KubeBlocks project
//
//This program is free software: you can redistribute it and/or modify
//it under the terms of the GNU Affero General Public License as published by
//the Free Software Foundation, either version 3 of the License, or
//(at your option) any later version.
//
//This program is distributed in the hope that it will be useful
//but WITHOUT ANY WARRANTY; without even the implied warranty of
//MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//GNU Affero General Public License for more details.
//
//You should have received a copy of the GNU Affero General Public License
//along with this program.  If not, see <http://www.gnu.org/licenses/>.

// required, command line input options for parameters and flags
options: {
	name:                    string
	namespace:               string
	opsRequestName:          string
	type:                    string
	typeLower:               string
	ttlSecondsAfterSucceed:  int
	clusterVersionRef:       string
	componentDefinitionName: string
	serviceVersion:          string
	component:               string
	instance:                string
	componentNames: [...string]
	rebuildInstanceFrom: [
		...{
			componentName: string
			backupName?:   string
			instances: [
				...{
					name:            string
					targetNodeName?: string
				},
			]
			envForRestore?: [
				...{
					name:  string
					value: string
				},
			]
		},
	]
	cpu:      string
	memory:   string
	replicas: int
	storage:  string
	vctNames: [...string]
	keyValues: [string]: {string | null}
	hasPatch:        bool
	fileContent:     string
	cfgTemplateName: string
	cfgFile:         string
	forceRestart:    bool
	force:           bool
	services: [
		...{
			name:        string
			serviceType: string
			annotations: {...}
		},
	]
	params: [
		...{
			name:  string
			value: string
		},
	]
	...
}

// define operation block,
#upgrade: {
	clusterVersionRef: options.clusterVersionRef
	if len(options.componentNames) > 0 {
		components: [ for _, cName in options.componentNames {
			componentName: cName
			if options.componentDefinitionName != "nil" {
				componentDefinitionName: options.componentDefinitionName
			}
			if options.serviceVersion != "nil" {
				serviceVersion: options.serviceVersion
			}
		}]
	}
}

// required, k8s api resource content
content: {
	apiVersion: "apps.kubeblocks.io/v1alpha1"
	kind:       "OpsRequest"
	metadata: {
		if options.opsRequestName == "" {
			generateName: "\(options.name)-\(options.typeLower)-"
		}
		if options.opsRequestName != "" {
			name: options.opsRequestName
		}
		namespace: options.namespace
		labels: {
			"app.kubernetes.io/instance":   options.name
			"app.kubernetes.io/managed-by": "kubeblocks"
		}
	}
	spec: {
		clusterName:            options.name
		type:                   options.type
		ttlSecondsAfterSucceed: options.ttlSecondsAfterSucceed
		force:                  options.force
		if options.type == "Upgrade" {
			upgrade: #upgrade
		}
		if options.type == "VolumeExpansion" {
			volumeExpansion: [ for _, cName in options.componentNames {
				componentName: cName
				volumeClaimTemplates: [ for _, vctName in options.vctNames {
					name:    vctName
					storage: options.storage
				}]
			}]
		}
		if options.type == "HorizontalScaling" {
			horizontalScaling: [ for _, cName in options.componentNames {
				componentName: cName
				replicas:      options.replicas
			}]
		}
		if options.type == "Restart" {
			restart: [ for _, cName in options.componentNames {
				componentName: cName
			}]
		}
		if options.type == "VerticalScaling" {
			verticalScaling: [ for _, cName in options.componentNames {
				componentName: cName
				requests: {
					if options.memory != "" {
						memory: options.memory
					}
					if options.cpu != "" {
						cpu: options.cpu
					}
				}
				limits: {
					if options.memory != "" {
						memory: options.memory
					}
					if options.cpu != "" {
						cpu: options.cpu
					}
				}
			}]
		}
		if options.type == "Reconfiguring" {
			if len(options.componentNames) == 1 {
				reconfigure: {
					componentName: options.componentNames[0]
					configurations: [ {
						name: options.cfgTemplateName
						if options.forceRestart {
							policy: "simple"
						}
						keys: [{
							key: options.cfgFile
							if options.fileContent != "" {
								fileContent: options.fileContent
							}
							if options.hasPatch {
								parameters: [ for k, v in options.keyValues {
									key:   k
									value: v
								}]
							}
						}]
					}]
				}
			}
			if len(options.componentNames) > 1 {
				reconfigures: [ for _, cName in options.componentNames {
					componentName: cName
					configurations: [ {
						name: options.cfgTemplateName
						if options.forceRestart {
							policy: "simple"
						}
						keys: [{
							key: options.cfgFile
							if options.fileContent != "" {
								fileContent: options.fileContent
							}
							if options.hasPatch {
								parameters: [ for k, v in options.keyValues {
									key:   k
									value: v
								}]
							}
						}]
					}]
				}]
			}
		}
		if options.type == "Expose" {
			expose: [ for _, cName in options.componentNames {
				componentName: cName
				if options.exposeEnabled == "true" {
					switch: "Enable"
				}
				if options.exposeEnabled == "false" {
					switch: "Disable"
				}
				services: [ for _, svc in options.services {
					name:        svc.name
					serviceType: svc.serviceType
					if len(svc.annotations) > 0 {
						annotations: svc.annotations
					}

				}]
			}]
		}
		if options.type == "Switchover" {
			switchover: [{
				if options.component == "" {
					componentName: options.componentNames[0]
				}
				if options.component != "" {
					componentName: options.component
				}
				if options.instance == "" {
					instanceName: "*"
				}
				if options.instance != "" {
					instanceName: options.instance
				}
			}]
		}
		if options.type == "RebuildInstance" {
			rebuildFrom: options.rebuildInstanceFrom
		}
		if options.type == "Custom" {
			custom: {
				opsDefinitionName: options.opsDefinitionName
				components: [
					{
						componentName: options.component
						parameters:    options.params
					},
				]
			}
		}
	}
}
