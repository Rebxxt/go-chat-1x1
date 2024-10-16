-- +goose Up
-- +goose StatementBegin
CREATE TABLE messages (
    sender varchar(32) not null,
    receiver varchar(32) not null,
    text VARCHAR(1000) not null,
    date timestamp default now() not null,
    constraint fk_sender foreign key (sender) references users(username),
    constraint fk_receiver foreign key (receiver) references users(username)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE messages;
-- +goose StatementEnd
