# Go : Errors

fixture-base 1.2.0

| Rule | Statement | Strength | Standing |
|---|---|---|---|
| go.errors.1 | Enveloppez une erreur avec ce qui était en cours. | must | enforced, blocked |
| go.errors.2 | Return an error and never panic in a library. | should | proposed |
| go.errors.3 | Check an error before the value it came with. | must | planned, trial, unproven |

## go.errors.1

Enveloppez une erreur avec ce qui était en cours.
