package auth

type LoginMethod interface {
	Validate() error
	Login() error
	AssumeRole() error
	Logout() error
}
