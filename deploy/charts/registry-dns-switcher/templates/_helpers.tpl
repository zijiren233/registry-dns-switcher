{{/*
Expand the name of the chart.
*/}}
{{- define "registry-dns-switcher.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "registry-dns-switcher.fullname" -}}
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

{{/*
Create chart name and version label.
*/}}
{{- define "registry-dns-switcher.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "registry-dns-switcher.labels" -}}
helm.sh/chart: {{ include "registry-dns-switcher.chart" . }}
{{ include "registry-dns-switcher.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "registry-dns-switcher.selectorLabels" -}}
app.kubernetes.io/name: {{ include "registry-dns-switcher.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name.
*/}}
{{- define "registry-dns-switcher.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "registry-dns-switcher.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Secret name.
*/}}
{{- define "registry-dns-switcher.secretName" -}}
{{- default (printf "%s-credentials" (include "registry-dns-switcher.fullname" .)) .Values.secret.existingSecret }}
{{- end }}
