package runtime

const (
	ReleaseOwner = "mosona-labs"
	ReleaseRepo  = "mosona-manager"
)

func ReleaseSlug() string {
	return ReleaseOwner + "/" + ReleaseRepo
}