-- Complete-value enforcement for the two source partial unique indexes.
-- Keep LONGTEXT and utf8mb4_0900_bin equality. No hash or prefix substitute.
-- Preflight fails explicitly on existing duplicate pairs. Never delete or
-- normalize source facts. Repeat after all four triggers close the DDL window.
CREATE TABLE catalog_option_uniqueness_audit (
 phase VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 scope VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 duplicate_pairs BIGINT NOT NULL CHECK(duplicate_pairs=0),
 PRIMARY KEY(phase,scope)
) ENGINE=InnoDB;
INSERT INTO catalog_option_uniqueness_audit(phase,scope,duplicate_pairs)
 SELECT 'before','title',COUNT(*) FROM catalog_options a JOIN catalog_options b
 ON a.id<b.id AND a.title=b.title AND a.deleted_at IS NULL AND b.deleted_at IS NULL AND a.is_exclusive=FALSE AND b.is_exclusive=FALSE
 UNION ALL
 SELECT 'before','value',COUNT(*) FROM catalog_option_values a JOIN catalog_option_values b
 ON a.id<b.id AND a.option_id=b.option_id AND a.value=b.value AND a.deleted_at IS NULL AND b.deleted_at IS NULL;

-- retail:trigger-begin
CREATE TRIGGER catalog_option_title_insert_unique BEFORE INSERT ON catalog_options FOR EACH ROW BEGIN
 DECLARE guard_version INT DEFAULT NULL;
 DECLARE conflicting VARCHAR(191) DEFAULT NULL;
 DECLARE CONTINUE HANDLER FOR NOT FOUND SET conflicting=NULL;
 SELECT version INTO guard_version FROM catalog_command_capabilities WHERE command='product.option-definitions' FOR UPDATE;
 IF guard_version IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option uniqueness guard missing'; END IF;
 IF NEW.deleted_at IS NULL AND NEW.is_exclusive=FALSE THEN
  SELECT id INTO conflicting FROM catalog_options WHERE deleted_at IS NULL AND is_exclusive=FALSE AND title=NEW.title LIMIT 1 FOR SHARE;
  IF conflicting IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option full value uniqueness violation'; END IF;
 END IF;
END;
-- retail:trigger-end

-- retail:trigger-begin
CREATE TRIGGER catalog_option_value_insert_unique BEFORE INSERT ON catalog_option_values FOR EACH ROW BEGIN
 DECLARE guard_version INT DEFAULT NULL;
 DECLARE conflicting VARCHAR(191) DEFAULT NULL;
 DECLARE CONTINUE HANDLER FOR NOT FOUND SET conflicting=NULL;
 SELECT version INTO guard_version FROM catalog_command_capabilities WHERE command='product.option-definitions' FOR UPDATE;
 IF guard_version IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option uniqueness guard missing'; END IF;
 IF NEW.deleted_at IS NULL AND NEW.option_id IS NOT NULL THEN
  SELECT id INTO conflicting FROM catalog_option_values WHERE deleted_at IS NULL AND option_id=NEW.option_id AND value=NEW.value LIMIT 1 FOR SHARE;
  IF conflicting IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option full value uniqueness violation'; END IF;
 END IF;
END;
-- retail:trigger-end

-- retail:trigger-begin
CREATE TRIGGER catalog_option_title_update_unique BEFORE UPDATE ON catalog_options FOR EACH ROW BEGIN
 DECLARE guard_version INT DEFAULT NULL;
 DECLARE conflicting VARCHAR(191) DEFAULT NULL;
 DECLARE CONTINUE HANDLER FOR NOT FOUND SET conflicting=NULL;
 SELECT version INTO guard_version FROM catalog_command_capabilities WHERE command='product.option-definitions' FOR UPDATE;
 IF guard_version IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option uniqueness guard missing'; END IF;
 IF NEW.deleted_at IS NULL AND NEW.is_exclusive=FALSE THEN
  SELECT id INTO conflicting FROM catalog_options WHERE deleted_at IS NULL AND is_exclusive=FALSE AND title=NEW.title AND id<>OLD.id LIMIT 1 FOR SHARE;
  IF conflicting IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option full value uniqueness violation'; END IF;
 END IF;
END;
-- retail:trigger-end

-- retail:trigger-begin
CREATE TRIGGER catalog_option_value_update_unique BEFORE UPDATE ON catalog_option_values FOR EACH ROW BEGIN
 DECLARE guard_version INT DEFAULT NULL;
 DECLARE conflicting VARCHAR(191) DEFAULT NULL;
 DECLARE CONTINUE HANDLER FOR NOT FOUND SET conflicting=NULL;
 SELECT version INTO guard_version FROM catalog_command_capabilities WHERE command='product.option-definitions' FOR UPDATE;
 IF guard_version IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option uniqueness guard missing'; END IF;
 IF NEW.deleted_at IS NULL AND NEW.option_id IS NOT NULL THEN
  SELECT id INTO conflicting FROM catalog_option_values WHERE deleted_at IS NULL AND option_id=NEW.option_id AND value=NEW.value AND id<>OLD.id LIMIT 1 FOR SHARE;
  IF conflicting IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='option full value uniqueness violation'; END IF;
 END IF;
END;
-- retail:trigger-end

INSERT INTO catalog_option_uniqueness_audit(phase,scope,duplicate_pairs)
 SELECT 'after','title',COUNT(*) FROM catalog_options a JOIN catalog_options b
 ON a.id<b.id AND a.title=b.title AND a.deleted_at IS NULL AND b.deleted_at IS NULL AND a.is_exclusive=FALSE AND b.is_exclusive=FALSE
 UNION ALL
 SELECT 'after','value',COUNT(*) FROM catalog_option_values a JOIN catalog_option_values b
 ON a.id<b.id AND a.option_id=b.option_id AND a.value=b.value AND a.deleted_at IS NULL AND b.deleted_at IS NULL;
