local M = {}

--[[
 Checks wether the response matches the expect block, returns boolean
 This will call fail on its own so should be used like:
 if not tools.expected() then
  return
 end
 So that the error is correctly printed, if you don't want fail called,
 pass true: if not tools.expected(true) then fail('custom error') end
--]]
function M.expected(dont_fail)
  local expect = rest.req.expect
  if expect == nil then return true end
  local res = rest.res
  if expect.status ~= res.status then
    if not dont_fail then
      fail(('unexpected status code %d != %d'):format(expect.status, res.status))
    end
    return false
  end
  if expect.body ~= nil and expect.body ~= '' and expect.body ~= res.body then
    if not dont_fail then
      fail(('body does not match expectation %s != %s'):format(expect.body, res.body))
    end
    return false
  end
  if expect.headers ~= nil then
    for k, v in pairs(expect.headers) do
      local values, finalkey = res:header_values(k)
      if values == nil then
        if not dont_fail then fail(('header %s does not present "%s" != "%s"'):format(finalkey)) end
        return false
      end
      local matches = false
      for _, value in ipairs(values) do
        if v == value then matches = true end
      end
      if not matches then
        -- small assumption that header is standalone for usablilty
        if not dont_fail then
          fail(('header %s does not match "%s" != "%s"'):format(finalkey, v, values[1]))
        end
        return false
      end
    end
  end
  return true
end

return M
