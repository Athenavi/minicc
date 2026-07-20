{{/*
Common name helpers
*/}}
{{- define "minicc-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "minicc-gateway.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "minicc-gateway.labels" -}}
helm.sh/chart: {{ include "minicc-gateway.name" . }}-{{ .Chart.Version }}
{{ include "minicc-gateway.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "minicc-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "minicc-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
