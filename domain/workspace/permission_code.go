package workspace

func PermissionRequiredCode(action Action) string {
	return string(action) + "_permission_required"
}
