package enrollment

import "strings"

func OwnerInitializationMessage(request OwnerInitializationRequest) []byte {
	return canonicalMessage("pfremote-owner-initialize/v1",
		request.FabricID,
		request.OwnerDeviceID,
		request.OwnerPublicKey,
		request.ClientVersion,
	)
}

func DeviceAuthorizationMessage(request DeviceAuthorizationRequest) []byte {
	return canonicalMessage("pfremote-device-authorize/v1",
		request.DeviceID,
		request.DeviceName,
		request.DevicePublicKey,
		request.ClientVersion,
		request.RequestID,
	)
}

func OwnerApprovalMessage(request OwnerApprovalRequest) []byte {
	return canonicalMessage("pfremote-owner-approve/v1",
		request.OwnerDeviceID,
		normalizeUserCode(request.UserCode),
		request.RequestID,
		request.ClientVersion,
	)
}

func OwnerRevocationMessage(request OwnerRevocationRequest) []byte {
	return canonicalMessage("pfremote-owner-revoke/v1",
		request.OwnerDeviceID,
		request.DeviceID,
		request.RequestID,
		request.ClientVersion,
	)
}

func VersionSyncMessage(request VersionSyncRequest) []byte {
	return canonicalMessage("pfremote-version-sync/v1",
		request.DeviceID,
		request.RequestID,
		request.ClientVersion,
	)
}

func ShellCapabilityPublishMessage(request ShellCapabilityPublishRequest) []byte {
	return canonicalMessage("pfremote-shell-capability-publish/v1",
		request.DeviceID,
		request.Binding.Signature,
		request.RequestID,
		request.ClientVersion,
	)
}

func ShellCapabilityListMessage(request ShellCapabilityListRequest) []byte {
	return canonicalMessage("pfremote-shell-capability-list/v1",
		request.OwnerDeviceID,
		request.RequestID,
		request.ClientVersion,
	)
}

func canonicalMessage(domain string, fields ...string) []byte {
	return []byte(domain + "\n" + strings.Join(fields, "\n"))
}
