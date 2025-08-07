package auth

type LoginMethod interface {
	Login() error
	Logout() error
	Validate() error
}
