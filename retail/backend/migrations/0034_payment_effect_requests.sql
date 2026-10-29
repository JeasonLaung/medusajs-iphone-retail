-- AES-GCM request snapshots, bound to the exact journal key and request digest.
-- Existing low-level receipts remain NULL and cannot use stored dispatch APIs.
ALTER TABLE payment_effect_journal
 ADD COLUMN request_envelope LONGBLOB NULL,
 ADD CONSTRAINT ck_payment_effect_envelope_size CHECK(request_envelope IS NULL OR OCTET_LENGTH(request_envelope) BETWEEN 34 AND 1048673);
