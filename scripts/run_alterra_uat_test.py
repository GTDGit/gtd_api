import importlib.util
import json
import pathlib
import unittest

import openpyxl


MODULE_PATH = pathlib.Path(__file__).with_name("run_alterra_uat.py")
SPEC = importlib.util.spec_from_file_location("run_alterra_uat", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AlterraUATScriptTests(unittest.TestCase):
    def test_make_alterra_request_payment_includes_reference_no(self):
        payload = json.loads(
            MODULE.make_alterra_request(
                "01428800700",
                "25",
                "payment",
                order_id="TRX-123",
                reference_no="REF-999",
            )
        )

        self.assertEqual(payload["order_id"], "TRX-123")
        self.assertEqual(payload["data"]["reference_no"], "REF-999")

    def test_validate_step_uses_alterra_http_and_rc(self):
        step = {
            "action": "Purchase",
            "expected_http": "406",
            "expected_rc": "20",
            "expected_result": "Failed",
        }
        scenario = {"flow": "prepaid_only"}
        results = {
            "purchase": {
                "provider_initial_http_status": 406,
                "provider_response_initial": json.dumps(
                    {
                        "order_id": "ORD-406",
                        "response_code": "20",
                        "status": "Failed",
                    }
                ),
            }
        }

        self.assertEqual(MODULE.validate_step(step, scenario, results), [])

    def test_validate_step_requires_actual_order_id_for_inquiry_based_purchase(self):
        step = {
            "action": "Purchase",
            "expected_http": "201",
            "expected_rc": "10",
            "expected_result": "Pending",
        }
        scenario = {"flow": "inquiry_purchase"}
        results = {
            "purchase": {
                "provider_initial_http_status": 201,
                "provider_response_initial": json.dumps(
                    {
                        "order_id": "",
                        "response_code": "10",
                        "status": "Pending",
                    }
                ),
            }
        }

        issues = MODULE.validate_step(step, scenario, results)

        self.assertIn(
            "Missing actual Alterra order_id for inquiry-based purchase/payment",
            issues,
        )

    def test_fill_scenario_rows_never_logs_transaction_zero(self):
        workbook = openpyxl.Workbook()
        sheet = workbook.active
        scenario = {
            "customer_no": "01428800700",
            "product_id": "25",
            "flow": "inquiry_purchase",
            "steps": [{"action": "Get detail/Callback", "row": 1}],
        }
        results = {
            "purchase": {
                "provider_response": json.dumps(
                    {
                        "transaction_id": 0,
                        "response_code": "10",
                        "status": "Pending",
                    }
                )
            }
        }

        MODULE.fill_scenario_rows(sheet, scenario, results, True, "")

        self.assertNotIn("/transaction/0", sheet.cell(1, 11).value or "")


if __name__ == "__main__":
    unittest.main()
