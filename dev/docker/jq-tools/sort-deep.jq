def sort_deep:
  if type == "array" then
    if length == 0 then []
    elif all(.[]; (type == "object") and has("name")) then
      sort_by(.name) | map(sort_deep)
    elif all(.[]; (type == "string") or (type == "number") or (type == "boolean")) then
      sort
    else
      map(sort_deep)
    end
  elif type == "object" then
    if length == 0 then {}
    else
      to_entries
      | sort_by(.key)
      | map({ (.key): (.value | sort_deep) })
      | add
    end
  else
    .
  end;
