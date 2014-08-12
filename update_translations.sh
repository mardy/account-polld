#!/bin/sh

sources=$(find . -name '*.go' | xargs)
domain='account-polld'
pot_file=po/$domain.pot
desktop=data/$domain.desktop
online_accounts_application=data/$domain.application

sed -e 's/description>/_description>/g' $online_accounts_application > $online_accounts_application.tr
sed -e 's/^Name=/_Name=/' $desktop > $desktop.tr

/usr/bin/intltool-extract --update --type=gettext/ini $desktop.tr $domain
/usr/bin/intltool-extract --update --type=gettext/xml $online_accounts_application.tr $domain

xgettext -o $pot_file \
 --add-comments \
 --from-code=UTF-8 \
 --c++ --qt --add-comments=TRANSLATORS \
 --keyword=Gettext --keyword=tr --keyword=tr:1,2 --keyword=N_ --keyword=_description \
 --package-name=$domain \
 --copyright-holder='Canonical Ltd.' \
 $sources $desktop.tr.h $online_accounts_application.tr.h

echo Removing $desktop.tr.h $online_accounts_application.tr.h
rm $desktop.tr.h $online_accounts_application.tr.h

if [ "$1" = "--commit" ] && [ -n "$(bzr status $pot_file)" ]; then
    echo Commiting $pot_file
    bzr commit -m "Updated translation template" $pot_file
fi
