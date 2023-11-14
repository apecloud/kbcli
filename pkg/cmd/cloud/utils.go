package cloud

import (
	"github.com/apecloud/kbcli/pkg/cmd/organization"
)

func IsCloud() bool {
	currentOrgAndContext, err := organization.GetCurrentOrgAndContext()
	if err == nil &&
		len(currentOrgAndContext.CurrentOrganization) > 0 &&
		len(currentOrgAndContext.CurrentContext) > 0 {
		return true
	}
	return false
}

func GetCurrentOrg() string {
	currentOrgAndContext, err := organization.GetCurrentOrgAndContext()
	if err != nil {
		return ""
	}
	return currentOrgAndContext.CurrentOrganization
}

func GetCurrentContext() string {
	currentOrgAndContext, err := organization.GetCurrentOrgAndContext()
	if err != nil {
		return ""
	}
	return currentOrgAndContext.CurrentContext
}

func GetToken() string {
	token, err := organization.GetToken()
	if err != nil {
		return ""
	}
	return token
}
