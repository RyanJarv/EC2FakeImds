set -eu

# Format of the results:
#   operation per line
#   disassociate:<assoicationId>:<tmpRouteTable>  -- disassociate assoicationId and delete tmpRouteTable
#   <origTableId>:<assoicationId>:<tmpRouteTable>  -- replace assoicationId with origTableId and delete tmpRouteTable
results="$(aws ec2 describe-route-tables --filter "Name=tag-key,Values=OrigRouteTableId" |jq -r '.RouteTables|.[].Tags|.[]| select(.Key == "OrigRouteTableId")|.Value'|uniq)"

echo "$results" | while read line; do
  origTableId="$(echo "$line"|cut -d: -f 1)";
  assoicationId="$(echo "$line"|cut -d: -f 2)";
  tmpRouteTable="$(echo "$line"|cut -d: -f 3)";

  echo "origTableId: ${origTableId}, assoicationId: ${assoicationId}, tmpRouteTable: ${tmpRouteTable}" 

  if [ "x$origTableId" == "xdisassociate" ]; then
    aws ec2 disassociate-route-table --association-id "$assoicationId" 
  else
    aws ec2 replace-route-table-association --association-id "$assoicationId" --route-table-id "$origTableId"
  fi

  aws ec2 delete-route-table --route-table-id "$tmpRouteTable"
done


