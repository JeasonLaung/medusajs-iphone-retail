# Product reviews

This local Medusa module powers the review section below product details and
the moderation widget on the admin product page. It uses the existing Medusa
SDK, customer authentication, workflows and PostgreSQL database.

The existing project had no review module or installed review plugin.
`@lambdacurry/medusa-product-reviews@1.4.0` was evaluated but not retained: its
published store validators use Zod 3 while this project uses Medusa's Zod 4,
and its store submission middleware does not require customer authentication.
The implementation follows Medusa's product-review customization approach:
https://docs.medusajs.com/resources/how-to-tutorials/tutorials/product-reviews

## Behavior

- A signed-in customer can submit one review per product, enforced by a unique
  database index. Reviews represent customer feedback, not verified purchases.
- The server derives the customer and public first name from authentication.
- New reviews are pending. Administrators publish, hide or return them to pending
  from the product details page. Only approved reviews enter public statistics.
- Store requests require a published product in the API key's sales channels.
- Public responses expose the author's first name, rating, title, content and
  date. Customer identifiers and email addresses are not exposed.
- Storefront and admin interfaces support English and Simplified Chinese.

## API

- `GET /store/product-reviews?product_id=...&offset=0&limit=5` returns public
  reviews, total count, average rating and the five rating counts.
- `POST /store/product-reviews` requires a customer session or bearer token and
  accepts `product_id`, integer `rating` (1–5), `title` (2–120 trimmed characters)
  and `content` (10–3000 trimmed characters).
- `GET /admin/product-reviews?product_id=...` lists all statuses for moderation.
- `POST /admin/product-reviews/:id` accepts a `status` of `pending`, `approved`
  or `rejected`. Admin authentication and existing supplier restrictions apply.

## Database and verification

The generated migration is included. On another installation, run
`pnpm exec medusa db:migrate` from `apps/backend`, then restart the backend.

Verify anonymous submissions fail, invalid ratings are rejected, repeat customer
submissions fail, pending reviews stay private, publishing updates public counts,
hiding removes the review from counts, and pagination retains the overall average.
Check both languages on the storefront and the admin product details page.
