-- +goose Up
CREATE TABLE gooser_probe_b (id int primary key);
-- +goose Down
DROP TABLE gooser_probe_b;
