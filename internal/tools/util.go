package tools

func IsValid(tools []Tool, toolName string) bool {
	for _, tool := range tools {
		if tool.Function.Name == toolName {
			return true
		}
	}
	return false
}

func GetNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Function.Name
	}
	return names
}
