CREATE TABLE item_sequence(singleton INTEGER PRIMARY KEY CHECK(singleton=1), value INTEGER NOT NULL);
INSERT INTO item_sequence VALUES(1,(SELECT COALESCE(MAX(id),0) FROM items));
