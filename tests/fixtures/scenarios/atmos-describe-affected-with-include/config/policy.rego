package spacelift

default allow = false

allow {
  input.session.login == "admin"
}
