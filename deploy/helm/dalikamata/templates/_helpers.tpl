{{/*
Expand the name of the chart.
*/}}
{{- define "dalikamata.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (e.g. by the DNS naming spec).
*/}}
{{- define "dalikamata.fullname" -}}
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
Chart label value: name-version.
*/}}
{{- define "dalikamata.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "dalikamata.labels" -}}
helm.sh/chart: {{ include "dalikamata.chart" . }}
{{ include "dalikamata.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (stable subset used by Services and Deployment matchLabels).
*/}}
{{- define "dalikamata.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dalikamata.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the NATS ClusterIP Service — used by all other services as --nats-host.
*/}}
{{- define "dalikamata.natsServiceName" -}}
{{- printf "%s-nats" (include "dalikamata.fullname" .) }}
{{- end }}

{{/*
Name of the Bitbucket Secret (existing or auto-created).
*/}}
{{- define "dalikamata.bitbucketSecretName" -}}
{{- .Values.ingest.bitbucket.existingSecret | default (printf "%s-bitbucket" (include "dalikamata.fullname" .)) }}
{{- end }}

{{/*
Name of the Jenkins Secret (existing or auto-created).
*/}}
{{- define "dalikamata.jenkinsSecretName" -}}
{{- .Values.ingest.jenkins.existingSecret | default (printf "%s-jenkins" (include "dalikamata.fullname" .)) }}
{{- end }}

{{/*
Name of the Grafana Secret (existing or auto-created).
*/}}
{{- define "dalikamata.grafanaSecretName" -}}
{{- .Values.grafana.existingSecret | default (printf "%s-grafana" (include "dalikamata.fullname" .)) }}
{{- end }}
