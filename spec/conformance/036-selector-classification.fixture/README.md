# 036 selector-classification

Parse-only classification of selectors against the `capture-baseline` selector profile ([SEP-0021](../../seps/)). `cases.json` is an array of `{"selector": string, "code": string}`. Each entry gives the selector and the code a profile parser reports for it: `""` when the selector is accepted, `selector-not-in-profile` when it is rejected.

Every implementation must produce the listed code for each selector. The code is normative; error messages (and wording) are not.
