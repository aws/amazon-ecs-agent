# TTime - Testable Time

TTime is a small package meant to be used in place of the Go std "time" package.
The driving goal of this package is to be testable by allowing a user to plug in
their own 'time' struct.

If no 'time' struct is configured, ttime will behave normally.

A testing shim, `ttime.TestTime`, is also provided which allows time to be
fast-forwarded and skipped.


This package does not provide a complete replacement to the 'time' package at
this time, preferring to cover the most common use-cases that include "Sleep"
and "Now".
