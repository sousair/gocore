-- +goose Up
CREATE TABLE gooser_probe (id int primary key);
-- +goose Down
DROP TABLE gooser_probe;
