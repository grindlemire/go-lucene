# Release Process

### Note this might be out of date, I have to figure out what is going on here

## Rules for release branches:

-   If you are releasing a new major version you need to branch off of master into a branch `release-branch.v#` (example `release-branch.v2` for a 2.x release)
-   If you are releasing a minor or patch update to an existing major release make sure to merge master into the release branch

## Rules for tagging and publishing the release

When you are ready to publish the release make sure you...

1. Merge your changes into the correct release branch.
2. Check out the release branch locally (example: `git pull origin release-branch.v3`)
3. Create a new tag for the specific release version you will publish (example: `git tag v3.0.1`)
4. Push the tag up to github (example: `git push origin v3.0.1`)
5. Check that the github action successfully finished and created a release
