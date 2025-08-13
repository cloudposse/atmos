package auth

type LoginMethod interface {
	Validate() error
	Login() error
	AssumeRole() error
	Logout() error
	getProfile() string
	getRegion() string
}
