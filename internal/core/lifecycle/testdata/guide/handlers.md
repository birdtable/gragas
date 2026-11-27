# Handler guide

This guide says how a service handler behaves. The intro carries no strength word.

## Logging

Handlers MUST log once per request.
Handlers MUST NOT log secrets.
Handlers SHOULD log at the edge.

## Errors

An error MUST be wrapped with context.
An error MUST be handled once.

## Retries

A retry MUST be bounded.
A retry MAY use jitter.

## Timeouts

Every call MUST carry a deadline.
A deadline MUST come from the caller and SHOULD be short.

## Naming

Exported names MUST NOT stutter.
Test names MUST say the case.
