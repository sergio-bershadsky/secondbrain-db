package sync

// ComputeDrift returns the DriftResult for one (doc, integration) pair.
//
// observedRemoteRev may be "" when the caller is doing a local-only check
// (no network). In that case, remote_drift is never reported.
func ComputeDrift(section SidecarSection, currentDocHash, observedRemoteRev string) DriftResult {
	if section.LastPush == nil {
		return ResultNeverPublished
	}
	localDrift := section.LastPush.DocHash != currentDocHash
	remoteDrift := observedRemoteRev != "" && observedRemoteRev != section.LastPush.RemoteRevision
	switch {
	case localDrift && remoteDrift:
		return ResultBothDrift
	case localDrift:
		return ResultLocalDrift
	case remoteDrift:
		return ResultRemoteDrift
	default:
		return ResultInSync
	}
}
