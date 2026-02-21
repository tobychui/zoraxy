#!/bin/bash

# Cd into zoraxy plugin directory
cd ../../src/mod/plugins/zoraxy_plugin/

# Add header to the documentation
echo "# Zoraxy Plugin APIs" >docs.md
echo "This API documentation is auto-generated from the Zoraxy plugin source code." >>docs.md
echo "" >>docs.md
echo "" >>docs.md
echo "<pre><code class='language-go'>" >>docs.md
go doc -all >>docs.md
echo "</code></pre>" >>docs.md

# Replace // import "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin" with
# // import "{{your_module_package_name_in_go.mod}}/mod/plugins/zoraxy_plugin"
sed -i 's|// import "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"|// import "{{your_module_package_name_in_go.mod}}/mod/plugins/zoraxy_plugin"|g' docs.md

# Move the generated docs to the plugins/html directory
mv docs.md "../../../../docs/plugins/docs/zoraxy_plugin API.md"

echo "Done generating Zoraxy plugin documentation."
