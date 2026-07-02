#!/bin/sh
set -eu

if [ "$#" -lt 5 ] || [ "$1" != "exec" ] || [ "$2" != "fixture-app" ] || [ "$3" != "php" ] || [ "$4" != "artisan" ]; then
  echo "unsupported docker shim invocation: $*" >&2
  exit 2
fi

shift 4
exec php /srv/app/artisan "$@"
