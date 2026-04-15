def remove_property(f):
  . as $in
  | if type == "object" then
      reduce keys[] as $key ({}; . + { ($key): ( ($in[$key] | remove_property(f)) ) } ) | f
    elif type == "array" then
      map( remove_property(f) ) | f
    else
      f
    end;

def remove_ids:
  remove_property(if type == "object" then del(.id) else . end);
