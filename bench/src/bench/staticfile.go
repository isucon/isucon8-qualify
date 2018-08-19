package bench

type StaticFile struct {
	Path string
	Size int64
	Hash string
}

var (
	StaticFiles = []*StaticFile{
		&StaticFile{"/css/admin.css", 684, "af8ea54a6883660979ab6e9d3b041a41"},
		&StaticFile{"/css/bootstrap.min.css", 140930, "a7022c6fa83d91db67738d6e3cd3252d"},
		&StaticFile{"/css/layout.css", 633, "200670917150073a849920414f900bb7"},
		&StaticFile{"/favicon.ico", 1092, "07b21a6c8984e04d108064c585411601"},
		&StaticFile{"/js/admin.js", 7214, "076a82aeda8e53aaf9d502f89cc4b77d"},
		&StaticFile{"/js/app.js", 7407, "7b12dc331513d061ba90435a14456ed0"},
		&StaticFile{"/js/bootstrap.bundle.min.js", 70682, "d70c474886678aebe3e9d91965dc8b62"},
		&StaticFile{"/js/fetch.min.js", 7337, "b72077f7f0fa3fc8f79a2fc57c15d827"},
		&StaticFile{"/js/jquery-3.3.1.slim.min.js", 69917, "99b0a83cf1b0b1e2cb16041520e87641"},
		&StaticFile{"/js/vue.min.js", 86452, "5283b86cbf48a538ee3cbebac633ccd4"},
	}
)

const (
	ExpectedIndexHash = 497858079
	ExpectedAdminHash = 2213621546
)
