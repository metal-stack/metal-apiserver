package api.v1.metalstack.io.authorization_test

import data.api.v1.metalstack.io.authorization
import rego.v1

public_visibilitymethods := ["/metalstack.api.v2.PublicService/List"]

public_visibility := {"public": {"/metalstack.api.v2.PublicService/List": true}}

test_public_visibility_with_token_allowed if {
	authorization.decision.allow with input as {
		"method": "/metalstack.api.v2.PublicService/List",
		"token": tokenv1,
		"request": {"project": "project-a"},
	}
		with data.methods as public_visibilitymethods
		with data.visibility as public_visibility
}

test_public_visibility_without_token_allowed if {
	authorization.decision.allow with input as {
		"method": "/metalstack.api.v2.PublicService/List",
		"token": null,
		"request": {},
	}
		with data.methods as public_visibilitymethods
		with data.visibility as public_visibility
}
