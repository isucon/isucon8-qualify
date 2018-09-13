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
		&StaticFile{"/css/layout.css", 707, "25d20a88af77ba832e0d25a99ebe67c3"},
		&StaticFile{"/favicon.ico", 1092, "07b21a6c8984e04d108064c585411601"},
		&StaticFile{"/js/admin.js", 8454, "f3739b2c9ba150e2a2c4f53d7194fea2"},
		&StaticFile{"/js/app.js", 10204, "43f9dccae02a8dab134b20c053e41f7b"},
		&StaticFile{"/js/bootstrap-waitingfor.min.js", 2074, "c6167b2ec19dc56b16aa94511a15964c"},
		&StaticFile{"/js/bootstrap.bundle.min.js", 70682, "d70c474886678aebe3e9d91965dc8b62"},
		&StaticFile{"/js/fetch.min.js", 7337, "b72077f7f0fa3fc8f79a2fc57c15d827"},
		&StaticFile{"/js/jquery-3.3.1.slim.min.js", 69917, "99b0a83cf1b0b1e2cb16041520e87641"},
		&StaticFile{"/js/vue.min.js", 86452, "5283b86cbf48a538ee3cbebac633ccd4"},
	}
)

const (
	ExpectedIndexHash = 888931047
	ExpectedAdminHash = 3940591906
)
