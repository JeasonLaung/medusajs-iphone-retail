-- Historical rows retain their numbers. Only an explicitly seeded allocator
-- may issue numbers for new orders; application startup never seeds it.
CREATE TABLE order_number_allocator (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 initialized BOOLEAN NOT NULL DEFAULT FALSE CHECK(initialized IN(FALSE,TRUE)),
 last_display_id BIGINT NOT NULL DEFAULT 0 CHECK(last_display_id BETWEEN 0 AND 2147483647),
 last_tick BIGINT NOT NULL DEFAULT 0 CHECK(last_tick>=0),
 seed_display_id BIGINT NULL,
 seed_tick BIGINT NULL,
 seed_sha256 BINARY(32) NULL,
 updated_at DATETIME(6) NOT NULL,
 CHECK((initialized=FALSE AND seed_display_id IS NULL AND seed_tick IS NULL AND seed_sha256 IS NULL)
    OR (initialized=TRUE AND seed_display_id IS NOT NULL AND seed_tick IS NOT NULL AND seed_sha256 IS NOT NULL
        AND seed_display_id>=0 AND seed_tick>=0 AND last_display_id>=seed_display_id AND last_tick>=seed_tick))
) ENGINE=InnoDB;
INSERT INTO order_number_allocator(id,version,initialized,last_display_id,last_tick,updated_at)
 VALUES(1,1,FALSE,0,0,UTC_TIMESTAMP(6));
