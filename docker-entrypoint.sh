#!/bin/sh
echo "$TZ" >  /etc/timezone
cp /usr/share/zoneinfo/$TZ   /etc/localtime
exec "$@"
