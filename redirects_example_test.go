package redirects

import (
	"encoding/json"
	"os"
)

func Example() {
	h := Must(ParseString(`
		# Implicit 301 redirects
		/home              /
		/blog/my-post.php  /blog/my-post
		/news              /blog
		/google            https://www.google.com

		# Redirect with a 301
		/home         /              301

		# Redirect with a 302
		/my-redirect  /              302

		# Rewrite a path
		/pass-through /index.html    200

		# Show a custom 404 for this path
		/ecommerce    /store-closed  404

		# Single page app rewrite
		/*    /index.html   200

		# Proxying
		/api/*  https://api.example.com/:splat  200

		# Query parameters
		/things type=photos /photos.html 200
		/things type=       /empty.html 200
		/things type=:thing /thing-:thing.html 200
		/things             /things.html 200
  
		# Multiple query parameters
		/stuff type=lost name=:name other=:ignore /other-stuff/:name.html 200

		# Query parameters with implicit 301
		/items id=:id /items/:id.html
  `))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(h)
	// Output:
	// 	[
	//   {
	//     "From": "/home",
	//     "FromQuery": null,
	//     "To": "/",
	//     "Status": 301
	//   },
	//   {
	//     "From": "/blog/my-post.php",
	//     "FromQuery": null,
	//     "To": "/blog/my-post",
	//     "Status": 301
	//   },
	//   {
	//     "From": "/news",
	//     "FromQuery": null,
	//     "To": "/blog",
	//     "Status": 301
	//   },
	//   {
	//     "From": "/google",
	//     "FromQuery": null,
	//     "To": "https://www.google.com",
	//     "Status": 301
	//   },
	//   {
	//     "From": "/home",
	//     "FromQuery": null,
	//     "To": "/",
	//     "Status": 301
	//   },
	//   {
	//     "From": "/my-redirect",
	//     "FromQuery": null,
	//     "To": "/",
	//     "Status": 302
	//   },
	//   {
	//     "From": "/pass-through",
	//     "FromQuery": null,
	//     "To": "/index.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/ecommerce",
	//     "FromQuery": null,
	//     "To": "/store-closed",
	//     "Status": 404
	//   },
	//   {
	//     "From": "/*",
	//     "FromQuery": null,
	//     "To": "/index.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/api/*",
	//     "FromQuery": null,
	//     "To": "https://api.example.com/:splat",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/things",
	//     "FromQuery": {
	//       "type": "photos"
	//     },
	//     "To": "/photos.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/things",
	//     "FromQuery": {
	//       "type": ""
	//     },
	//     "To": "/empty.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/things",
	//     "FromQuery": {
	//       "type": ":thing"
	//     },
	//     "To": "/thing-:thing.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/things",
	//     "FromQuery": null,
	//     "To": "/things.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/stuff",
	//     "FromQuery": {
	//       "name": ":name",
	//       "other": ":ignore",
	//       "type": "lost"
	//     },
	//     "To": "/other-stuff/:name.html",
	//     "Status": 200
	//   },
	//   {
	//     "From": "/items",
	//     "FromQuery": {
	//       "id": ":id"
	//     },
	//     "To": "/items/:id.html",
	//     "Status": 301
	//   }
	// ]
}
