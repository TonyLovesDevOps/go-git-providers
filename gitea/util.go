/*
Copyright 2020 The Flux CD contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gitea

import (
	"fmt"
	"net/http"

	"code.gitea.io/sdk/gitea"

	"github.com/fluxcd/go-git-providers/gitprovider"
	"github.com/fluxcd/go-git-providers/validation"
)

const (
	alreadyExistsMagicString = "name already exists on this account"
)

// TODO: Guard better against nil pointer dereference panics in this package, also
// validate data coming from the server

// validateUserRepositoryRef makes sure the UserRepositoryRef is valid for Gitea's usage.
func validateUserRepositoryRef(ref gitprovider.UserRepositoryRef, expectedDomain string) error {
	// Make sure the RepositoryRef fields are valid
	if err := validation.ValidateTargets("UserRepositoryRef", ref); err != nil {
		return err
	}
	// Make sure the type is valid, and domain is expected
	return validateIdentityFields(ref, expectedDomain)
}

// validateOrgRepositoryRef makes sure the OrgRepositoryRef is valid for Gitea's usage.
func validateOrgRepositoryRef(ref gitprovider.OrgRepositoryRef, expectedDomain string) error {
	// Make sure the RepositoryRef fields are valid
	if err := validation.ValidateTargets("OrgRepositoryRef", ref); err != nil {
		return err
	}
	// Make sure the type is valid, and domain is expected
	return validateIdentityFields(ref, expectedDomain)
}

// validateOrganizationRef makes sure the OrganizationRef is valid for Gitea's usage.
func validateOrganizationRef(ref gitprovider.OrganizationRef, expectedDomain string) error {
	// Make sure the OrganizationRef fields are valid
	if err := validation.ValidateTargets("OrganizationRef", ref); err != nil {
		return err
	}
	// Make sure the type is valid, and domain is expected
	return validateIdentityFields(ref, expectedDomain)
}

// validateUserRef makes sure the UserRef is valid for Gitea's usage.
func validateUserRef(ref gitprovider.UserRef, expectedDomain string) error {
	// Make sure the OrganizationRef fields are valid
	if err := validation.ValidateTargets("UserRef", ref); err != nil {
		return err
	}
	// Make sure the type is valid, and domain is expected
	return validateIdentityFields(ref, expectedDomain)
}

// validateIdentityFields makes sure the type of the IdentityRef is supported, and the domain is as expected.
func validateIdentityFields(ref gitprovider.IdentityRef, expectedDomain string) error {
	// Make sure the expected domain is used
	if ref.GetDomain() != expectedDomain {
		return fmt.Errorf("domain %q not supported by this client: %w", ref.GetDomain(), gitprovider.ErrDomainUnsupported)
	}
	// Make sure the right type of identityref is used
	switch ref.GetType() {
	case gitprovider.IdentityTypeOrganization, gitprovider.IdentityTypeUser:
		return nil
	case gitprovider.IdentityTypeSuborganization:
		return fmt.Errorf("gitea doesn't support sub-organizations: %w", gitprovider.ErrNoProviderSupport)
	}
	return fmt.Errorf("invalid identity type: %v: %w", ref.GetType(), gitprovider.ErrInvalidArgument)
}

// handleHTTPError checks the type of err, and returns typed variants of it
// However, it _always_ keeps the original error too, and just wraps it in a MultiError
// The consumer must use errors.Is and errors.As to check for equality and get data out of it.
func handleHTTPError(res *gitea.Response, err error) error {
	// Short-circuit quickly if possible, allow always piping through this function
	if err == nil {
		return nil
	}
	if res != nil {
		httpErr := gitprovider.HTTPError{
			Response:         res.Response,
			ErrorMessage:     err.Error(),
			Message:          err.Error(),
			DocumentationURL: "",
		}
		// Check for invalid credentials, and return a typed error in that case
		if httpErr.Response.StatusCode == http.StatusForbidden ||
			httpErr.Response.StatusCode == http.StatusUnauthorized {
			return validation.NewMultiError(err,
				&gitprovider.InvalidCredentialsError{HTTPError: httpErr},
			)
		}
		// Check for 404 Not Found
		if httpErr.Response.StatusCode == http.StatusNotFound {
			return validation.NewMultiError(err, gitprovider.ErrNotFound)
		}

		if httpErr.Response.StatusCode == http.StatusConflict {
			return validation.NewMultiError(err, gitprovider.ErrAlreadyExists)
		}

		// Check 422 Unprocessable Entity for already exists
		if httpErr.Response.StatusCode == http.StatusUnprocessableEntity {
			return validation.NewMultiError(err, gitprovider.ErrAlreadyExists)
		}

		return validation.NewMultiError(err, &httpErr)
	}
	// Do nothing, just pipe through the unknown err
	return err
}

// // allPages runs fn for each page, expecting a HTTP request to be made and returned during that call.
// // allPages expects that the data is saved in fn to an outer variable.
// // allPages calls fn as many times as needed to get all pages, and modifies opts for each call.
// // There is no need to wrap the resulting error in handleHTTPError(err), as that's already done.
func allPages(opts *gitea.ListOptions, fn func() (*gitea.Response, error)) error {
	opts.Page = 1
	for {
		resp, err := fn()
		if err != nil {
			return handleHTTPError(resp, err)
		}
		if resp == nil || resp.Header.Get("Link") == "" {
			return nil
		}
		opts.Page += 1
	}
}

// validateAPIObject creates a Validatior with the specified name, gives it to fn, and
// depending on if any error was registered with it; either returns nil, or a MultiError
// with both the validation error and ErrInvalidServerData, to mark that the server data
// was invalid.
func validateAPIObject(name string, fn func(validation.Validator)) error {
	v := validation.New(name)
	fn(v)
	// If there was a validation error, also mark it specifically as invalid server data
	if err := v.Error(); err != nil {
		return validation.NewMultiError(err, gitprovider.ErrInvalidServerData)
	}
	return nil
}
