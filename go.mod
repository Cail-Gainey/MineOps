module github.com/Cail-Gainey/MineOps

go 1.26.5

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/wailsapp/wails/v3 v3.0.0-alpha2.117
	github.com/zalando/go-keyring v0.2.6
)

require (
	github.com/WissCore/go-sqlcipher/v4 v4.15.1
	golang.org/x/crypto v0.54.0
	golang.org/x/sys v0.47.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.2
)

require (
	al.essio.dev/pkg/shellescape v1.6.0 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/mattn/go-sqlite3 => ./internal/infrastructure/sqlcipher/driver
