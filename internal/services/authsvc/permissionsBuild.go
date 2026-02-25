// internal/services/authsvc/permissionsBuild.go
// Permissions are no longer bundled into signIn/introspect responses.
// Clients fetch menu permissions via GET /orgs/menu/access after selecting an org.
package authsvc
