# ydyCron
golang cron &amp;&amp; web controllers


    CREATE TABLE "task" (
    "id"  INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    "name"  text(50),
    "settime"  text(50),
    "cmd"  text(200),
    "desc"  text,
    "status"  INTEGER DEFAULT 1,
    "ctime"  INTEGER
    );


     CREATE TABLE "task_history" (
    "id"  INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    "task_id"  INTEGER,
    "start_time"  INTEGER,
    "end_time"  INTEGER,
    "output"  text
    );


