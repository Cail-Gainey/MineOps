package apperror

// Code is a stable machine-readable MineOps error identifier.
type Code string

const (
	CodeInternal                             Code = "internal.unexpected"
	CodeValidationInvalidArgument            Code = "validation.invalid_argument"
	CodeValidationRequired                   Code = "validation.required"
	CodeValidationConflict                   Code = "validation.conflict"
	CodeSSHConnectionFailed                  Code = "ssh.connection_failed"
	CodeSSHAuthenticationFailed              Code = "ssh.authentication_failed"
	CodeSSHHostKeyRejected                   Code = "ssh.host_key_rejected"
	CodeSFTPTransferFailed                   Code = "sftp.transfer_failed"
	CodeSFTPPathRejected                     Code = "sftp.path_rejected"
	CodeIOReadFailed                         Code = "io.read_failed"
	CodeIOWriteFailed                        Code = "io.write_failed"
	CodeIONotFound                           Code = "io.not_found"
	CodeIOPermissionDenied                   Code = "io.permission_denied"
	CodeHTTPConnectionFailed                 Code = "http.connection_failed"
	CodeHTTPStatusFailed                     Code = "http.status_failed"
	CodeHTTPResponseTooLarge                 Code = "http.response_too_large"
	CodeArtifactChecksumMismatch             Code = "artifact.checksum_mismatch"
	CodeArtifactSizeExceeded                 Code = "artifact.size_exceeded"
	CodeInstallationPreflightFailed          Code = "installation.preflight_failed"
	CodeInstallationDirectoryConflict        Code = "installation.directory_conflict"
	CodeInstallationJavaFailed               Code = "installation.java_failed"
	CodeInstallationArtifactFailed           Code = "installation.artifact_failed"
	CodeInstallationEULAFailed               Code = "installation.eula_failed"
	CodeInstallationFirstStartFailed         Code = "installation.first_start_failed"
	CodeInstallationRegistrationFailed       Code = "installation.registration_failed"
	CodeProcessStartFailed                   Code = "process.start_failed"
	CodeProcessExitFailed                    Code = "process.exit_failed"
	CodeProcessCancelled                     Code = "process.cancelled"
	CodeCryptoKeyUnavailable                 Code = "crypto.key_unavailable"
	CodeCryptoDecryptFailed                  Code = "crypto.decrypt_failed"
	CodeCryptoIntegrityFailed                Code = "crypto.integrity_failed"
	CodeAgentUnavailable                     Code = "agent.unavailable"
	CodeAgentProtocolUnsupported             Code = "agent.protocol_unsupported"
	CodeAgentAuthenticationFailed            Code = "agent.authentication_failed"
	CodeMetricCollectionFailed               Code = "metric.collection_failed"
	CodeMetricQueryFailed                    Code = "metric.query_failed"
	CodeMetricCapacityExceeded               Code = "metric.capacity_exceeded"
	CodeSparkUnavailable                     Code = "spark.unavailable"
	CodeSparkUnsupported                     Code = "spark.unsupported"
	CodeSparkParseFailed                     Code = "spark.parse_failed"
	CodeFirewallUnsupported                  Code = "firewall.unsupported"
	CodeFirewallPermissionDenied             Code = "firewall.permission_denied"
	CodeFirewallApplyFailed                  Code = "firewall.apply_failed"
	CodeDesktopUpdateMetadataUnauthenticated Code = "desktop_update.metadata_unauthenticated"
	CodeDesktopUpdateUnsupported             Code = "desktop_update.unsupported"
	CodeDesktopUpdateTargetNotWritable       Code = "desktop_update.target_not_writable"
	CodeDesktopUpdateCancelled               Code = "desktop_update.cancelled"
	CodeDesktopUpdateApplyFailed             Code = "desktop_update.apply_failed"
)

var validCodes = map[Code]struct{}{
	CodeInternal: {}, CodeValidationInvalidArgument: {}, CodeValidationRequired: {}, CodeValidationConflict: {},
	CodeSSHConnectionFailed: {}, CodeSSHAuthenticationFailed: {}, CodeSSHHostKeyRejected: {},
	CodeSFTPTransferFailed: {}, CodeSFTPPathRejected: {}, CodeIOReadFailed: {}, CodeIOWriteFailed: {},
	CodeIONotFound: {}, CodeIOPermissionDenied: {}, CodeHTTPConnectionFailed: {}, CodeHTTPStatusFailed: {},
	CodeHTTPResponseTooLarge: {}, CodeArtifactChecksumMismatch: {}, CodeArtifactSizeExceeded: {},
	CodeInstallationPreflightFailed: {}, CodeInstallationDirectoryConflict: {}, CodeInstallationJavaFailed: {},
	CodeInstallationArtifactFailed: {}, CodeInstallationEULAFailed: {}, CodeInstallationFirstStartFailed: {},
	CodeInstallationRegistrationFailed: {},
	CodeProcessStartFailed:             {}, CodeProcessExitFailed: {},
	CodeProcessCancelled: {}, CodeCryptoKeyUnavailable: {}, CodeCryptoDecryptFailed: {}, CodeCryptoIntegrityFailed: {},
	CodeAgentUnavailable: {}, CodeAgentProtocolUnsupported: {}, CodeAgentAuthenticationFailed: {},
	CodeMetricCollectionFailed: {}, CodeMetricQueryFailed: {}, CodeMetricCapacityExceeded: {}, CodeSparkUnavailable: {}, CodeSparkUnsupported: {},
	CodeSparkParseFailed: {}, CodeFirewallUnsupported: {}, CodeFirewallPermissionDenied: {}, CodeFirewallApplyFailed: {},
	CodeDesktopUpdateMetadataUnauthenticated: {}, CodeDesktopUpdateUnsupported: {}, CodeDesktopUpdateTargetNotWritable: {},
	CodeDesktopUpdateCancelled: {}, CodeDesktopUpdateApplyFailed: {},
}

// String returns the stable serialized error code.
func (c Code) String() string {
	return string(c)
}

// Valid reports whether the code belongs to the registered global error set.
func (c Code) Valid() bool {
	_, ok := validCodes[c]
	return ok
}
