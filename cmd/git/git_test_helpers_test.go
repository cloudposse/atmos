package git

func gitTestArgs(args ...string) []string {
	base := []string{
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"-c", "gpg.format=openpgp",
	}
	return append(base, args...)
}
