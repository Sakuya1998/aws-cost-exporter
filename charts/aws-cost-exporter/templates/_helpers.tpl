{{- define "aws-cost-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "aws-cost-exporter.validateDeploymentStrategy" -}}
{{- if ne (int .Values.replicaCount) 1 -}}
{{- fail "replicaCount must equal 1; coordinated multi-replica refresh is not supported" -}}
{{- end -}}
{{- if ne .Values.deploymentStrategy.type "RollingUpdate" -}}
{{- fail "deploymentStrategy.type must equal RollingUpdate" -}}
{{- end -}}
{{- if ne (int .Values.deploymentStrategy.rollingUpdate.maxSurge) 0 -}}
{{- fail "deploymentStrategy.rollingUpdate.maxSurge must equal 0" -}}
{{- end -}}
{{- if ne (int .Values.deploymentStrategy.rollingUpdate.maxUnavailable) 1 -}}
{{- fail "deploymentStrategy.rollingUpdate.maxUnavailable must equal 1" -}}
{{- end -}}
{{- end }}

{{- define "aws-cost-exporter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "aws-cost-exporter.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "aws-cost-exporter.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{ include "aws-cost-exporter.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "aws-cost-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aws-cost-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "aws-cost-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "aws-cost-exporter.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "aws-cost-exporter.configMapName" -}}
{{- default (include "aws-cost-exporter.fullname" .) .Values.config.existingConfigMap }}
{{- end }}

{{- define "aws-cost-exporter.validatePort" -}}
{{- if not .Values.config.existingConfigMap -}}
{{- $listenAddress := dig "server" "listen_address" "" .Values.config.data | toString -}}
{{- $listenPort := regexFind "[0-9]+$" $listenAddress -}}
{{- if or (eq $listenPort "") (ne $listenPort (.Values.service.targetPort | toString)) -}}
{{- fail "service.targetPort must match config.data.server.listen_address" -}}
{{- end -}}
{{- end -}}
{{- end }}
