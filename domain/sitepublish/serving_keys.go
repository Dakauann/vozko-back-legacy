package sitepublish

const deploymentKeyPrefix = "deploy:"

func DeploymentKey(deploymentID string) string {
	return deploymentKeyPrefix + deploymentID
}
