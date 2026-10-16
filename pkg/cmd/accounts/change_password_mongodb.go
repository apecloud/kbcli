package accounts

import (
	"encoding/json"
	"fmt"
	"strings"
)

const mongoPasswordShellCommand = `if command -v mongosh >/dev/null 2>&1; then
	exec mongosh --host 127.0.0.1 --port 27017 --quiet
elif command -v mongo >/dev/null 2>&1; then
	exec mongo --host 127.0.0.1 --port 27017 --quiet
else
	echo "neither mongosh nor mongo is installed" >&2
	exit 127
fi
`

type mongoPasswordChanger struct {
	config passwordChangerConfig
}

func (c *mongoPasswordChanger) ChangePassword(request passwordChangeRequest) error {
	database := c.config.Database
	if database == "" {
		database = defaultMongoPasswordDatabase
	}

	command := []string{"sh", "-c", mongoPasswordShellCommand}
	script := buildMongoPasswordScript(database, request.AuthUser, request.AuthPassword, request.TargetUser, request.NewPassword)
	return executePasswordCommand(c.config, request.PodName, command, strings.NewReader(script), true, "MongoDB client")
}

func buildMongoPasswordScript(database, authUser, authPassword, targetUser, newPassword string) string {
	return fmt.Sprintf(
		"var auth = db.getSiblingDB(%s).auth(%s, %s); if (!auth) { throw new Error(\"authentication failed\"); } var result = db.getSiblingDB(%s).runCommand({updateUser: %s, pwd: %s}); if (!result || result.ok !== 1) { printjson(result); throw new Error(\"updateUser failed\"); }",
		quoteMongoJavaScriptString(database),
		quoteMongoJavaScriptString(authUser),
		quoteMongoJavaScriptString(authPassword),
		quoteMongoJavaScriptString(database),
		quoteMongoJavaScriptString(targetUser),
		quoteMongoJavaScriptString(newPassword),
	)
}

func quoteMongoJavaScriptString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
