{{/* vim: set filetype=mustache: */}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Return the cronhpa image name
*/}}
{{- define "controller.image" -}}
{{- include "common.images.image" (dict "imageRoot" .Values.controller.image "global" .Values.global) -}}
{{- end -}}

{{/*
Return the cleanup jb image name
*/}}
{{- define "cleanup.image" -}}
{{- include "common.images.image" (dict "imageRoot" .Values.cleanup.image "global" .Values.global) -}}
{{- end -}}