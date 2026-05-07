{{- define "ghacs.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "ghacs.fullname" -}}
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
{{- end }}

{{- define "ghacs.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "ghacs.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ghacs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "ghacs.labels" -}}
helm.sh/chart: {{ include "ghacs.chart" . }}
{{ include "ghacs.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "ghacs.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "ghacs.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end }}

{{- define "ghacs.pvcName" -}}
{{ include "ghacs.fullname" . }}-data
{{- end }}

{{- define "ghacs.multipleReplicas" -}}
{{- if or (and .Values.autoscaling.enabled (gt (.Values.autoscaling.maxReplicas | int) 1)) (gt (.Values.replicaCount | int) 1) -}}
true
{{- else -}}
false
{{- end -}}
{{- end }}

{{- define "ghacs.pvcEnabled" -}}
{{- if kindIs "bool" .Values.persistentVolumeClaim.enabled -}}
{{- .Values.persistentVolumeClaim.enabled -}}
{{- else -}}
{{- or (eq .Values.config.storage.driver "filesystem") (eq .Values.config.db.driver "sqlite") -}}
{{- end -}}
{{- end }}

{{- define "ghacs.pvcAccessModes" -}}
{{- if and (eq (include "ghacs.multipleReplicas" .) "true") (eq .Values.config.storage.driver "filesystem") -}}
- ReadWriteMany
{{- else -}}
{{ toYaml .Values.persistentVolumeClaim.accessModes }}
{{- end -}}
{{- end }}

{{- define "ghacs.validate" -}}
{{- if and (eq .Values.config.db.driver "sqlite") (eq (include "ghacs.multipleReplicas" .) "true") -}}
{{- fail "SQLite cannot be used with multiple replicas. Switch to postgres or mysql." -}}
{{- end -}}
{{- end }}

{{- define "ghacs.env" -}}
- name: PORT
  value: "3000"
- name: API_BASE_URL
  value: {{ default (printf "http://%s.%s.svc.cluster.local:%v" (include "ghacs.fullname" .) .Release.Namespace .Values.service.port) .Values.config.apiBaseUrl | quote }}
- name: ENABLE_DIRECT_DOWNLOADS
  value: {{ .Values.config.enableDirectDownloads | quote }}
- name: CACHE_CLEANUP_OLDER_THAN_DAYS
  value: {{ .Values.config.cacheCleanupOlderThanDays | quote }}
{{- if .Values.config.disableCleanupJobs }}
- name: DISABLE_CLEANUP_JOBS
  value: "true"
{{- end }}
{{- if .Values.config.debug }}
- name: DEBUG
  value: "true"
{{- end }}
{{- if .Values.config.managementApiKey }}
- name: MANAGEMENT_API_KEY
  value: {{ .Values.config.managementApiKey | quote }}
{{- end }}
{{- with .Values.config.diskPressureMinFreeBytes }}
- name: DISK_PRESSURE_MIN_FREE_BYTES
  value: {{ . | quote }}
{{- end }}
{{- with .Values.config.diskPressureTargetFreeBytes }}
- name: DISK_PRESSURE_TARGET_FREE_BYTES
  value: {{ . | quote }}
{{- end }}
- name: STORAGE_DRIVER
  value: {{ .Values.config.storage.driver | quote }}
{{- if eq .Values.config.storage.driver "filesystem" }}
- name: STORAGE_FILESYSTEM_PATH
  value: {{ .Values.config.storage.filesystem.path | quote }}
{{- else if eq .Values.config.storage.driver "s3" }}
{{- with .Values.config.storage.s3 }}
{{- if .bucket }}
- name: STORAGE_S3_BUCKET
  value: {{ .bucket | quote }}
{{- end }}
{{- if .region }}
- name: AWS_REGION
  value: {{ .region | quote }}
{{- end }}
{{- if .endpointUrl }}
- name: AWS_ENDPOINT_URL
  value: {{ .endpointUrl | quote }}
{{- end }}
{{- if .accessKeyId }}
- name: AWS_ACCESS_KEY_ID
  value: {{ .accessKeyId | quote }}
{{- end }}
{{- if .secretAccessKey }}
- name: AWS_SECRET_ACCESS_KEY
  value: {{ .secretAccessKey | quote }}
{{- end }}
{{- end }}
{{- else if eq .Values.config.storage.driver "gcs" }}
{{- with .Values.config.storage.gcs }}
{{- if .bucket }}
- name: STORAGE_GCS_BUCKET
  value: {{ .bucket | quote }}
{{- end }}
{{- if .serviceAccountKey }}
- name: STORAGE_GCS_SERVICE_ACCOUNT_KEY
  value: {{ .serviceAccountKey | quote }}
{{- end }}
{{- if .endpoint }}
- name: STORAGE_GCS_ENDPOINT
  value: {{ .endpoint | quote }}
{{- end }}
{{- end }}
{{- end }}
- name: DB_DRIVER
  value: {{ .Values.config.db.driver | quote }}
{{- if eq .Values.config.db.driver "sqlite" }}
- name: DB_SQLITE_PATH
  value: {{ .Values.config.db.sqlite.path | quote }}
{{- else if eq .Values.config.db.driver "postgres" }}
{{- with .Values.config.db.postgres }}
{{- if .url }}
- name: DB_POSTGRES_URL
  value: {{ .url | quote }}
{{- else }}
{{- if .database }}
- name: DB_POSTGRES_DATABASE
  value: {{ .database | quote }}
{{- end }}
{{- if .host }}
- name: DB_POSTGRES_HOST
  value: {{ .host | quote }}
{{- end }}
{{- if .port }}
- name: DB_POSTGRES_PORT
  value: {{ .port | quote }}
{{- end }}
{{- if .user }}
- name: DB_POSTGRES_USER
  value: {{ .user | quote }}
{{- end }}
{{- if .password }}
- name: DB_POSTGRES_PASSWORD
  value: {{ .password | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- else if eq .Values.config.db.driver "mysql" }}
{{- with .Values.config.db.mysql }}
{{- if .database }}
- name: DB_MYSQL_DATABASE
  value: {{ .database | quote }}
{{- end }}
{{- if .host }}
- name: DB_MYSQL_HOST
  value: {{ .host | quote }}
{{- end }}
{{- if .port }}
- name: DB_MYSQL_PORT
  value: {{ .port | quote }}
{{- end }}
{{- if .user }}
- name: DB_MYSQL_USER
  value: {{ .user | quote }}
{{- end }}
{{- if .password }}
- name: DB_MYSQL_PASSWORD
  value: {{ .password | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
