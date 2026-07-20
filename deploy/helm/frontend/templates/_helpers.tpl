{{- define "minicc-frontend.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "minicc-frontend.fullname" -}}
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

{{- define "minicc-frontend.labels" -}}
helm.sh/chart: {{ include "minicc-frontend.name" . }}-{{ .Chart.Version }}
{{ include "minicc-frontend.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "minicc-frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "minicc-frontend.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
