#!/bin/bash
go test  -coverprofile=c.out  -v | tee test.out && go tool cover -html=c.out
