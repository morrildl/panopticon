package panopticon

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // register sqlite3 driver
)

var schemaVersion = 2
var schemaStatements = [][]string{
	[]string{
		"create table Version (Version int not null unique, Updated datetime default current_timestamp, rowid integer primary key check (rowid=1))",
		"create trigger v_u_ts after update on version begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",
		"create trigger v_i_ts after insert on version begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",

		"create table Settings (Key text not null unique, Value text not null default '', Updated datetime default current_timestamp)",
		"create trigger s_u_ts after update on Settings begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",
		"create trigger s_i_ts after insert on Settings begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",

		"create table Cameras (Name text not null, ID text not null unique, Address text not null, Diurnal int default 0, AspectRatio text not null default '16x9', Timelapse text not null default 'none', ImageURL text not null, RTSPURL text not null default '', Updated datetime default current_timestamp)",
		"create trigger c_u_ts after update on Cameras begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",
		"create trigger c_i_ts after insert on Cameras begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",

		"create table Users (Name text not null default '', Email text not null unique, Updated datetime default current_timestamp)",
		"create trigger u_u_ts after update on Users begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",
		"create trigger u_i_ts after insert on Users begin update version set Updated=strftime('%Y-%m-%dT%H:%M:%f', 'now') where rowid=NEW.rowid; end",

		"insert into Version (Version) values (1)",
	},
	[]string{
		"update Version set Version=2",
	},
	[]string{
		"alter table Cameras add Latitude real not null default 0.0",
		"alter table Cameras add Longitude real not null default 0.0",
		"update Version set Version=3",
	},
	[]string{
		"alter table Settings add Scope text not null default 'SYSTEM'",
		"update Version set Version=4",
	},
	[]string{
		"alter table Cameras add Dewarp int not null default 0",
		"update Version set Version=5",
	},
}

func (sys *SystemConfig) getDB() *sql.DB {
	cxn, err := sql.Open("sqlite3", sys.SqlitePath)
	if err != nil {
		panic(err)
	}
	return cxn
}

func (sys *SystemConfig) writeDatabaseByQuery(query string, params ...interface{}) {
	cxn := sys.getDB()
	defer cxn.Close()

	_, err := cxn.Exec(query, params...)
	if err != nil {
		panic(err)
	}
}

func (sys *SystemConfig) initSchema() {
	cxn := sys.getDB()
	defer cxn.Close()

	curVersion := 0
	if rows, err := cxn.Query("select name from sqlite_master where type='table' and name='Version'"); err != nil {
		panic(err)
	} else {
		exists := rows.Next()
		rows.Close()
		if exists {
			if rows, err := cxn.Query("select Version from Version"); err != nil {
				panic(err)
			} else {
				if !rows.Next() {
					panic("version table exists but has no row")
				}
				rows.Scan(&curVersion)
				rows.Close()
			}
			if curVersion == 0 {
				panic("version exists but is set to 0")
			}
		}
	}
	for i := curVersion; i < len(schemaStatements); i++ {
		for _, q := range schemaStatements[i] {
			if _, err := cxn.Exec(q); err != nil {
				panic(err)
			}
		}
	}
}
