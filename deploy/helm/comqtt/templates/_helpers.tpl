{{/*
Expand the name of the chart.
*/}}
{{- define "comqtt.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name.
*/}}
{{- define "comqtt.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Headless Service name (cluster mode).
*/}}
{{- define "comqtt.headlessName" -}}
{{- printf "%s-headless" (include "comqtt.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Chart label string.
*/}}
{{- define "comqtt.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Standard labels.
*/}}
{{- define "comqtt.labels" -}}
helm.sh/chart: {{ include "comqtt.chart" . }}
{{ include "comqtt.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: broker
app.kubernetes.io/part-of: comqtt
comqtt.io/mode: {{ .Values.mode | quote }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "comqtt.selectorLabels" -}}
app.kubernetes.io/name: {{ include "comqtt.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
ServiceAccount name.
*/}}
{{- define "comqtt.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "comqtt.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Resolve the image tag, falling back to .Chart.AppVersion. Refuse `latest`.
*/}}
{{- define "comqtt.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if eq $tag "latest" -}}
{{- fail "image.tag=latest is not permitted; pin a real version (set image.tag to a specific release or rely on .Chart.AppVersion)." -}}
{{- end -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{/*
Resolve which broker binary to invoke.
*/}}
{{- define "comqtt.binary" -}}
{{- if eq .Values.mode "cluster" -}}/comqtt-cluster{{- else -}}/comqtt{{- end -}}
{{- end -}}

{{/*
Cluster Raft quorum minimum.
*/}}
{{- define "comqtt.raftQuorum" -}}
{{- div (add .Values.replicaCount 1) 2 -}}
{{- end -}}

{{/*
Render the rendered config (single-mode — used as-is) or the template config
(cluster mode — placeholder substitution happens at runtime).
*/}}
{{- define "comqtt.configYaml" -}}
{{- toYaml .Values.config | nindent 0 -}}
{{- end -}}
