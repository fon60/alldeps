## MODIFIED Requirements

### Requirement: Installed package listing
In project mode for a Composer project, the system SHALL list every installed package from the project's installed-package metadata (`vendor/composer/installed.json`) with its exact installed version. Direct dependencies (those declared in `composer.json`) and packages pulled in automatically as transitive dependencies SHALL both be listed; automatically installed packages SHALL carry the automatic marker in their flag. Platform requirements (for example `php` or `ext-*`) are not installed packages and SHALL NOT be listed as rows. When the project has no installed vendor directory, the list SHALL be empty and operations SHALL remain available.

#### Scenario: Direct dependencies listed with exact versions
- **WHEN** a Composer project declares three direct dependencies and its installed metadata records their resolved versions
- **THEN** the list shows those three direct packages, each with the package name and its exact installed version (not the declared constraint)

#### Scenario: Transitive dependencies listed and marked automatic
- **WHEN** a Composer project's installed metadata includes five transitive packages pulled in by its direct dependencies
- **THEN** the list also shows those five transitive packages, each carrying the automatic marker `A`

#### Scenario: Platform requirements are not listed
- **WHEN** the project's `composer.json` declares platform requirements such as `php` or `ext-mbstring`
- **THEN** those are not shown as rows, since they are not installed packages

#### Scenario: Fresh project without vendor directory
- **WHEN** a Composer project has `composer.json` but no `vendor/` directory
- **THEN** the list is empty and marking packages for installation remains possible
