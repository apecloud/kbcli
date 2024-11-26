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
	name:                         string
	namespace:                    string
	clusterName:                  string | *null
	depth:                        int64 | *null
	locale:                       string | *null
	celStateEvaluationExpression: string | *null
}

// required, k8s api resource content
content: {
	apiVersion: "trace.kubeblocks.io/v1"
	kind:       "ReconciliationTrace"
	metadata: {
		name:      options.name
		namespace: options.namespace
	}
	spec: {
		if options.clusterName != null {
			targetObject: {
				namespace: options.namespace
				name:      options.clusterName
			}
		}
		if options.depth != null {
			depth: options.depth
		}
		if options.locale != null {
			locale: options.locale
		}
		if options.celStateEvaluationExpression != null {
			stateEvaluationExpression: {
				celExpression: {
					expression: options.celStateEvaluationExpression
				}
			}
		}
	}
}
