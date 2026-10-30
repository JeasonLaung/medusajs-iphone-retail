CREATE TABLE buyback_history_claims (
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 actor_id VARCHAR(80) NOT NULL,
 evidence_note LONGTEXT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(request_id) REFERENCES buyback_history_stores(request_id),
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id),
 INDEX buyback_claim_customer(customer_id,request_id)
) ENGINE=InnoDB;
CREATE SQL SECURITY INVOKER VIEW buyback_account_access AS
 SELECT b.request_id,b.customer_id,b.store_id FROM customer_buyback_submissions b
 JOIN buyback_requests r ON r.id=b.request_id AND r.customer_id=b.customer_id
 UNION ALL
 SELECT c.request_id,c.customer_id,h.store_id FROM buyback_history_claims c
 JOIN buyback_history_stores h ON h.request_id=c.request_id
 WHERE NOT EXISTS(SELECT 1 FROM customer_buyback_submissions b WHERE b.request_id=c.request_id);
