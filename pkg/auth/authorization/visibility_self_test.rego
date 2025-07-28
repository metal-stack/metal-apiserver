package api.v1.metalstack.io.authorization_test

import data.api.v1.metalstack.io.authorization
import rego.v1

self_visibilitymethods := ["/metalstack.api.v2.ImageService/List"]

self_visibility := {"self": {"/metalstack.api.v2.ImageService/List": true}}

test_self_visibility_with_token_allowed if {
	authorization.decision.allow with input as {
		"method": "/metalstack.api.v2.ImageService/List",
		"token": tokenv1,
		"request": {},
		"permissions": {"project-a": ["/metalstack.api.v2.ImageService/List"]},
	}
		with data.methods as self_visibilitymethods
		with data.visibility as self_visibility
}

test_self_visibility_without_token_allowed if {
	not authorization.decision.allow with input as {
		"method": "/metalstack.api.v2.ImageService/List",
		"token": null,
		"request": {},
	}
		with data.methods as self_visibilitymethods
		with data.visibility as self_visibility
}
