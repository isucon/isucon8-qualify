package bench

type StaticFile struct {
	Path string
	Size int64
	Hash string
}

var (
	StaticFiles = []*StaticFile{
		&StaticFile{"/favicon.ico", 1092, "07b21a6c8984e04d108064c585411601"},
		&StaticFile{"/css/bootstrap.min.css", 140930, "a7022c6fa83d91db67738d6e3cd3252d"},
		&StaticFile{"/css/layout.css", 426, "69b5c07f3aa24cc2efac2903c694e9be"},
		&StaticFile{"/js/jquery-3.3.1.slim.min.js", 69917, "99b0a83cf1b0b1e2cb16041520e87641"},
		&StaticFile{"/js/bootstrap.bundle.min.js", 70682, "d70c474886678aebe3e9d91965dc8b62"},
		&StaticFile{"/js/vue.min.js", 86452, "5283b86cbf48a538ee3cbebac633ccd4"},
		&StaticFile{"/js/fetch.min.js", 7337, "b72077f7f0fa3fc8f79a2fc57c15d827"},
		&StaticFile{"/js/app.js", 7284, "ea40880f8dbde79b31e0a492f48612c8"},
	}
)
